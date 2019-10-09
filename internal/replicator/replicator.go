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
	"time"
)

const TopicContentUpdate = "content-update"
const TopicNamespaceDelete = "namespace.delete"

type Replicator struct {
	KubeClient     *kubernetes.Clientset
	Namespace      string
	SecretName     string
	Targets        map[string]bool
	TargetTemplate string

	contentBuffer  map[string][]byte
	contentRWMutex sync.RWMutex

	TargetSecretName   map[string]string
	targetSecretMutex  sync.RWMutex
	cleanTargetSecrets bool

	wg   sync.WaitGroup
	quit chan bool
}

func NewReplicator(conf *util.Configuration, clientset *kubernetes.Clientset) *Replicator {
	r := &Replicator{
		KubeClient:         clientset,
		Namespace:          conf.WatchedNamespace,
		SecretName:         conf.WatchedSecretName,
		TargetTemplate:     conf.TargetNameTemplate,
		cleanTargetSecrets: conf.CleanNamespaces,

		quit:             make(chan bool, 0),
		Targets:          map[string]bool{},
		TargetSecretName: map[string]string{},
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

func (r *Replicator) Run(exit chan bool) {
	defer r.cleanup()

	r.wg.Add(1)
	go r.secretWatcher()

	r.wg.Add(1)
	go r.namespaceWatcher()

	select {
	case <-r.quit:
		close(exit)
	case <-exit:
		close(r.quit)
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
			klog.Infof("received event for secret %s: %v", secret.Name, event.Type)
			if secret.Name != r.SecretName {
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				r.contentRWMutex.Lock()
				r.contentBuffer = secret.Data
				r.contentRWMutex.Unlock()

				util.Publish(TopicContentUpdate, time.Now())
			case watch.Deleted:
				klog.Errorf("watched secret %s/%s has been deleted, synchronisation has been suspended.", r.Namespace, r.SecretName)
			case watch.Bookmark:
				// do nothing here
			case watch.Error:
				klog.Fatal("error occured while watching, exiting")
			}
			continue
		case <-r.quit:
			return
		}
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
			klog.Infof("received event for namespace %s: %v", namespace.Name, event.Type)
			if len(r.Targets) > 0 && !r.Targets[namespace.Name] {
				// if we have target namespaces and it is not listed in the map, skip it.
				continue
			}

			switch event.Type {
			case watch.Added:
				r.wg.Add(1)
				go r.syncNamespace(namespace)
			case watch.Deleted:
				klog.Infof("removing syncer for namespace %v", namespace.Name)
				util.Publish(TopicNamespaceDelete, namespace.Name)
			case watch.Modified, watch.Bookmark:
				// do nothing here
			case watch.Error:
				klog.Fatal("error occurred while watching, exiting")
			}
			continue
		case <-r.quit:
			return
		}
	}
}

func (r *Replicator) syncNamespace(namespace *v1.Namespace) {
	defer r.wg.Done()

	var first sync.Once
	nsName := namespace.Name

	doUpdate := func() {
		name := func() string {
			r.targetSecretMutex.RLock()
			name, ok := r.TargetSecretName[nsName]
			r.targetSecretMutex.RUnlock()
			if !ok {
				name = fmt.Sprintf("%v%08x", r.TargetTemplate, crc32.ChecksumIEEE([]byte(nsName)))
				r.targetSecretMutex.Lock()
				r.TargetSecretName[nsName] = name
				r.targetSecretMutex.Unlock()
				klog.Infof("syncing to secret %v/%v", nsName, name)
			}
			return name
		}()

		r.contentRWMutex.RLock()
		defer r.contentRWMutex.RUnlock()

		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsName,
				Labels:    map[string]string{"managed-by-injector": "true"},
			},
			Data: r.contentBuffer,
		}
		first.Do(func() { _, _ = r.KubeClient.CoreV1().Secrets(nsName).Create(secret) })
		_, err := r.KubeClient.CoreV1().Secrets(nsName).Update(secret)
		if err != nil {
			klog.Errorf("error while updating secret %s/%s %v", nsName, name, err)
		}
	}

	// Do an update on creation to initialize the secret.
	doUpdate()

	ch := make(chan util.Event)
	unsubContent := util.Subscribe(TopicContentUpdate, ch)
	unsubNamespace := util.Subscribe(TopicNamespaceDelete, ch)
	for {
		select {
		case event := <-ch:
			switch event.Topic {
			case TopicContentUpdate:
				doUpdate()
			case TopicNamespaceDelete:
				if name, ok := event.Message.(string); ok && name == nsName {
					goto exit
				}
			}
		case <-r.quit:
			goto exit
		}
	}

exit:
	unsubContent()
	unsubNamespace()
	close(ch)
}

func (r *Replicator) cleanup() {
	r.wg.Wait()

	if r.cleanTargetSecrets {
		for ns, name := range r.TargetSecretName {
			err := r.KubeClient.CoreV1().Secrets(ns).Delete(name, &metav1.DeleteOptions{})
			if err != nil {
				klog.Errorf("error while deleting %s/%s: %v", ns, name, err)
			}
		}
	}
}
