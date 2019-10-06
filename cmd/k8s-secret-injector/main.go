package main

import (
	"fmt"
	"github.com/spf13/pflag"
	"k8s.io/klog"
	"magnax.ca/k8s-secret-injector/internal/kube"
	"magnax.ca/k8s-secret-injector/internal/replicator"
	"magnax.ca/k8s-secret-injector/internal/util"
	"magnax.ca/k8s-secret-injector/version"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func parseFlags(args []string) (bool, *util.Configuration, error) {
	conf := &util.Configuration{
	}

	flags := pflag.NewFlagSet("", pflag.ExitOnError)
	showVersion := flags.Bool("version", false, "Print version information and exit")

	flags.StringVar(&conf.ApiserverHost, "master", "", "URL of the k8s apiserver")
	flags.StringVar(&conf.KubeconfigFile, "kubeconfig", "", "path to the kubeconfig file")

	flags.StringVar(&conf.WatchedNamespace, "namespace", "default", "Namespace of the source secret")
	flags.StringVar(&conf.WatchedSecretName, "secret", "", "Name of the source secret")

	flags.StringSliceVar(&conf.TargetNamespaces, "target-namespaces", []string{}, "Namespace(s) where the secret should be replicated, comma-separated. leave blank for all")
	flags.StringVar(&conf.TargetNameTemplate, "target-template", "", "Template to use when generating the replicated secret names, leave blank for a generated template")

	noClean := flags.Bool("no-clean", false,"Do not clean namespaces of the created secrets")

	if err := flags.Parse(args); err != nil {
		return false, nil, err
	}

	if *showVersion {
		return true, nil, nil
	}

	if conf.WatchedSecretName == "" || conf.WatchedNamespace == "" {
		return false, nil, fmt.Errorf("both the source namespace and secret must be specified")
	}

	conf.CleanNamespaces = !*noClean

	return false, conf, nil
}

func main() {
	fmt.Printf("%v\n\t%v\n", os.Args[0], version.String())

	exitOnVersion, conf, err := parseFlags(os.Args[1:])
	if exitOnVersion {
		os.Exit(0)
	}
	if err != nil {
		klog.Fatal(err)
	}

	kubeclient, err := kube.NewClient(conf.ApiserverHost, conf.KubeconfigFile)
	if err != nil {
		klog.Fatal(err)
	}

	quit := make(chan bool)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		close(quit)
	}()

	wg := sync.WaitGroup{}
	replicatorApp := replicator.NewReplicator(conf, kubeclient)

	wg.Add(1)
	go func() {
		defer wg.Done()
		replicatorApp.Run(quit)
	}()

	wg.Wait()
	klog.Info("exiting")
}
