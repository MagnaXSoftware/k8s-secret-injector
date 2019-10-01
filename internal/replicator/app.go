package replicator

import (
	"fmt"
	"hash/crc32"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"magnax.ca/k8s-secret-injector/internal/util"
	"sync"
)

type Replicator struct {
	KubeClient     *kubernetes.Clientset
	Namespace      string
	SecretName     string
	Targets        map[string]bool
	TargetTemplate string

	contentBuffer  map[string][]byte
	contentRWMutex sync.RWMutex

	targetSyncerChans  map[string]chan bool
	TargetSecretName   map[string]string
	targetSecretMutex  sync.RWMutex
	cleanTargetSecrets bool

	wg       sync.WaitGroup
	quitChan chan bool
}

func NewReplicator(conf *util.Configuration, clientset *kubernetes.Clientset) *Replicator {
	r := &Replicator{
		KubeClient:     clientset,
		Namespace:      conf.WatchedNamespace,
		SecretName:     conf.WatchedSecret,
		TargetTemplate: conf.TargetTemplate,

		quitChan:           make(chan bool, 0),
		Targets:            map[string]bool{},
		targetSyncerChans:  map[string]chan bool{},
		TargetSecretName:   map[string]string{},
		cleanTargetSecrets: true,
	}

	if len(conf.TargetNamespaces) != 0 {
		for _, target := range conf.TargetNamespaces {
			r.Targets[target] = true
		}
	}

	if len(r.TargetTemplate) == 0 {
		r.TargetTemplate = fmt.Sprintf("%s-sync-", r.SecretName)
	}

	return r
}

func (r *Replicator) Run(exitChan chan bool) {
	defer r.Cleanup()

	r.wg.Add(1)
	go r.secretWatcher()

	r.wg.Add(1)
	go r.namespaceWatcher()

	select {
	case <-r.quitChan:
		close(exitChan)
	case <-exitChan:
		close(r.quitChan)
	}
}

func (r *Replicator) secretWatcher() {
	defer r.wg.Done()

	watcher, err := r.KubeClient.CoreV1().Secrets(r.Namespace).Watch(metav1.ListOptions{})
	if err != nil {
		klog.Fatal(err)
	}
	resultChan := watcher.ResultChan()

	for {
		select {
		case event := <-resultChan:
			secret, ok := event.Object.(*v1.Secret)
			if !ok {
				continue
			}
			klog.Infof("received new secret: %v", secret.Name)
			if secret.Name != r.SecretName {
				continue
			}
			switch event.Type {
			case watch.Added:
				fallthrough
			case watch.Modified:
				func() {
					r.contentRWMutex.Lock()
					defer r.contentRWMutex.Unlock()

					r.contentBuffer = secret.Data
				}()
				r.notifySyncers()
			case watch.Deleted:
				klog.Errorf("watched secret %s/%s has been deleted, synchronisation has been suspended.", r.Namespace, r.SecretName)
			case watch.Bookmark:
				// do nothing here
			case watch.Error:
				klog.Fatal("error occured while watching, exiting")
			}
			continue
		case <-r.quitChan:
			return
		}
	}
}

func (r *Replicator) notifySyncers() {
	for _, ch := range r.targetSyncerChans {
		ch <- true
	}
}

func (r *Replicator) namespaceWatcher() {
	defer r.wg.Done()

	watcher, err := r.KubeClient.CoreV1().Namespaces().Watch(metav1.ListOptions{})
	if err != nil {
		klog.Fatal(err)
	}
	resultChan := watcher.ResultChan()

	for {
		select {
		case event := <-resultChan:
			namespace, ok := event.Object.(*v1.Namespace)
			if !ok {
				continue
			}
			klog.Infof("received new namespace: %v", namespace.Name)
			if len(r.Targets) > 0 && !r.Targets[namespace.Name] {
				// if we have target namespaces and it is not listed in the map, skip it.
				continue
			}
			switch event.Type {
			case watch.Added:
				if _, ok := r.targetSyncerChans[namespace.Name]; ok {
					klog.Warning("received Added event for namespace %v, but we are already syncing the namespace", namespace.Name)
					continue
				}
				r.targetSyncerChans[namespace.Name] = make(chan bool, 1)
				r.wg.Add(1)
				go syncNamespace(r, namespace)
				r.targetSyncerChans[namespace.Name] <- true
			case watch.Modified:
				// do nothing here
			case watch.Deleted:
				klog.Infof("removing syncer for namespace %v", namespace.Name)
				close(r.targetSyncerChans[namespace.Name])
				delete(r.targetSyncerChans, namespace.Name)
			case watch.Bookmark:
				// do nothing here
			case watch.Error:
				klog.Fatal("error occured while watching, exiting")
			}
			continue
		case <-r.quitChan:
			return
		}
	}
}

func syncNamespace(r *Replicator, namespace *v1.Namespace) {
	defer r.wg.Done()

	first := true

	ch := r.targetSyncerChans[namespace.Name]
	for {
		select {
		case _, open := <-ch:
			if !open {
				klog.Infof("stopping sync on namespace %v", namespace.Name)
				break
			}
			r.targetSecretMutex.RLock()
			name, ok := r.TargetSecretName[namespace.Name]
			r.targetSecretMutex.RUnlock()
			if !ok {
				name = fmt.Sprintf("%v%08x", r.TargetTemplate, crc32.ChecksumIEEE([]byte(namespace.Name)))
				r.targetSecretMutex.Lock()
				r.TargetSecretName[namespace.Name] = name
				r.targetSecretMutex.Unlock()
				klog.Infof("syncing to secret %v/%v", namespace.Name, name)
			}
			r.contentRWMutex.RLock()
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace.Name,
					Labels:    map[string]string{"managed-by-injector": "true"},
				},
				Data: r.contentBuffer,
			}
			if first {
				_, _ = r.KubeClient.CoreV1().Secrets(namespace.Name).Create(secret)
				first = false
			}
			_, err := r.KubeClient.CoreV1().Secrets(namespace.Name).Update(secret)
			if err != nil {
				klog.Errorf("error while updating secret %s/%s %v", namespace.Name, name, err)
			}

			r.contentRWMutex.RUnlock()
		case <-r.quitChan:
			return
		}
	}
}

func (r *Replicator) Cleanup() {
	r.wg.Wait()

	if r.cleanTargetSecrets {
		for ns, name := range r.TargetSecretName {
			err := r.KubeClient.CoreV1().Secrets(ns).Delete(name, &metav1.DeleteOptions{})
			if err != nil {
				klog.Error("error while deleting %s/%s: %v", ns, name, err)
			}
		}
	}
}
