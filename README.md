[![Build Status](https://travis-ci.org/magaldima/intercom.svg?branch=master)](https://travis-ci.org/magaldima/intercom)
# intercom

`intercom` is a containerized plugin system over gRPC. My goal was to be able to deploy Kubernetes `Deployments` or `StatefulSets` along with a `Service` that would act as a blackbox interface defined by the gRPC protocol.

The [go-plugin](https://github.com/hashicorp/go-plugin) project provided for a lot of the initial inspiration and direction of this project, however I wanted to make it native to Kubernetes by leveraging `Pods` and `Services`. 

## Future Enhancements
Here's some wishful thinking for this project:
- the protocol wouldn't even have to be gRPC
- the protocol would be built at a much lower level such as the container runtime level (e.g. Docker, rocket). These runtimes would define an interface of the container by a similar method as `Expose` in Docker.