module github.com/openHPI/poseidon

go 1.22.4

require (
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/getsentry/sentry-go v0.28.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.3
	github.com/hashicorp/nomad v1.8.1
	github.com/hashicorp/nomad/api v0.0.0-20240624153520-bbdc8b7fa758
	github.com/influxdata/influxdb-client-go/v2 v2.13.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.9.0
	golang.org/x/sys v0.22.0
	gopkg.in/yaml.v3 v3.0.1
)

replace (
	// Some Hashicorp dependencies still use the old package name (and version). See https://github.com/hashicorp/nomad/issues/11826.
	github.com/armon/go-metrics => github.com/hashicorp/go-metrics v0.5.3
	// Nomad is not compatible with newer versions of HCLv2 yet. See https://github.com/hashicorp/nomad/issues/11826.
	github.com/hashicorp/hcl/v2 => github.com/hashicorp/hcl/v2 v2.20.2-0.20240517235513-55d9c02d147d
)

require (
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/apparentlymart/go-cidr v1.1.0 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/armon/go-metrics v0.5.3 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fatih/color v1.17.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.3 // indirect
	github.com/go-jose/go-jose/v4 v4.0.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/gojuno/minimock/v3 v3.3.13 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/hashicorp/consul/api v1.29.1 // indirect
	github.com/hashicorp/cronexpr v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-bexpr v0.1.14 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-cty-funcs v0.0.0-20240510212344-9599f7024f07 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-immutable-radix/v2 v2.1.0 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-plugin v1.6.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.7 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.8 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-set/v2 v2.1.0 // indirect
	github.com/hashicorp/go-sockaddr v1.0.6 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hashicorp/hcl/v2 v2.21.0 // indirect
	github.com/hashicorp/memberlist v0.5.1 // indirect
	github.com/hashicorp/raft v1.7.0 // indirect
	github.com/hashicorp/raft-autopilot v0.2.0 // indirect
	github.com/hashicorp/serf v0.10.2-0.20240320153621-5d32001edfaa // indirect
	github.com/hashicorp/vault/api v1.14.0 // indirect
	github.com/hashicorp/yamux v0.1.1 // indirect
	github.com/influxdata/line-protocol v0.0.0-20210922203350-b1ad95c89adf // indirect
	github.com/lufia/plan9stats v0.0.0-20240513124658-fba389f38bae // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.61 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-testing-interface v1.14.2-0.20210821155943-2d9075ca8770 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/hashstructure v1.1.0 // indirect
	github.com/mitchellh/pointerstructure v1.2.1 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/oapi-codegen/runtime v1.1.1 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/sean-/seed v0.0.0-20170313163322-e2103e2c3529 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zclconf/go-cty v1.14.4 // indirect
	github.com/zclconf/go-cty-yaml v1.0.3 // indirect
	golang.org/x/crypto v0.24.0 // indirect
	golang.org/x/exp v0.0.0-20240613232115-7f521ea00fb8 // indirect
	golang.org/x/mod v0.18.0 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.22.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240624140628-dc46fd24d27d // indirect
	google.golang.org/grpc v1.64.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	oss.indeed.com/go/libtime v1.6.0 // indirect
)
