# Software Requirement Specification

This file aims to collect the various requirements of the software. This will be used to ensure that the system built matches the given spec.

## Description

k8s-secret-injector is a software that will automatically mount a given secret to all pods in a kubernetes cluster.
It will replicate a source secret to various namespaces, and mount this secret on all new pods created in those namespaces.

The replication is necessary as kubernetes does not allow secrets to be mounted across namespaces.

## Requirements

### Functional

The software MUST synchronize a source secret to various target namespaces.

The software MUST synchronize modifications of the source secret to the destination secrets.

The software MUST NOT synchronize the deletion of the source secret to the destination secrets.

The software MUST allow for the deletion of the source secret while in operation.

The software MUST allow for the creation of the source secret while in operation.

The software MUST resume synchronization of the source secret when it is deleted then re-created.


The software SHOULD allow filtering of target namespaces.

The software MUST allow replication to target namespaces that are created during the software's operation.

The software MUST allow for the deletion of target namespaces while in operation.

The software MUST permit the modification of the name template used when creating the destination secrets.

The software MUST support removal of the destination secrets on exit.

The software MUST allow for the removal of the destination secrets to be turned off.


The software MUST mount the replicated secret on all containers of new pods in the target namespaces.

The software SHOULD permit exclusion from the replicated secret mount.

The software MUST allow specification of the secret's mount path.

The software MAY allow customisation of the secret's mount path for individual pods.

### Non-functional

The software MUST run on kubernetes clusters.

The software SHOULD be distributed as a docker image.

The software MUST be compatible with multiple certified kubernetes distributions.

The software SHOULD run on non-certified common kubernetes distributions.

The software MUST be compatible with RBAC-enabled kubernetes clusters.

The software SHOULD make TLS optional (if using a MutatingAdmissionController).

The software SHOULD be written in go (golang).  
As Kubernetes is built in go, writing this software in the same language gets us the best API access.