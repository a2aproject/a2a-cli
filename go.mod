module github.com/a2aproject/a2a-cli

go 1.26.0

require (
	github.com/a2aproject/a2a-go v0.3.15
	github.com/a2aproject/a2a-go/v2 v2.8.0
	github.com/google/go-cmp v0.7.0
	github.com/joho/godotenv v1.5.1
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	golang.org/x/sys v0.49.0
	golang.org/x/term v0.47.0
	google.golang.org/grpc v1.84.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261012220534-4341edf3f941 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261012220534-4341edf3f941 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

// Temporary: builds against the a2aclient/oauth package on the oauth-manager
// branch of mykytanetipa/a2a-go. Remove this replace, and require the a2a-go
// release that ships a2aclient/oauth, before the next a2a-cli release:
// `go install github.com/a2aproject/a2a-cli@latest` refuses a module with a
// replace directive.
replace github.com/a2aproject/a2a-go/v2 => github.com/mykytanetipa/a2a-go/v2 v2.0.0-20261108233611-e0b9420fded5
