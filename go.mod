module github.com/chhsia0/skycfg

go 1.14

require (
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/kylelemons/godebug v0.0.0-20170820004349-d65d576e9348
	go.starlark.net v0.0.0-20190604130855-6ddc71c0ba77
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/gengo v0.0.0-20200728071708-7794989d0000
)

replace github.com/kylelemons/godebug => github.com/jmillikin-stripe/godebug v0.0.0-20180620173319-8279e1966bc1
