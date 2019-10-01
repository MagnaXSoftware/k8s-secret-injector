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
	flags.StringVar(&conf.WatchedSecret, "secret", "", "Name of the source secret")

	flags.StringSliceVar(&conf.TargetNamespaces, "target-namespaces", []string{}, "Namespace(s) where the secret should be replicated, comma-separated. leave blank for all")
	flags.StringVar(&conf.TargetTemplate, "target-template", "", "Template to use when generating the replicated secret names, leave blank for a generated template")

	if *showVersion {
		return true, nil, nil
	}

	if err := flags.Parse(args); err != nil {
		return false, nil, err
	}

	if conf.WatchedSecret == "" || conf.WatchedNamespace == "" {
		return false, nil, fmt.Errorf("both the source namespace and secret must be specified")
	}

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

	exitChan := make(chan bool)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		close(exitChan)
	}()

	replicatorApp := replicator.NewReplicator(conf, kubeclient)

	go replicatorApp.Run(exitChan)

	select {
	case <-exitChan:
	}

	klog.Info("exiting")
}
