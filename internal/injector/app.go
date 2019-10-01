package injector

import "magnax.ca/k8s-secret-injector/internal/util"

type Injector struct {

}

func NewInjector(conf *util.Configuration) *Injector {
	in := &Injector{}

	return in
}

func (in *Injector) Run(exitChan chan bool) {
	select {
	case <-exitChan:
		return
	}
}
