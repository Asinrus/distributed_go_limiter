module github.com/asinrus/distributed_go_limiter/examples

go 1.26.5

require (
	github.com/asinrus/distributed_go_limiter/core v0.0.0
	github.com/asinrus/distributed_go_limiter/etcdstore v0.0.0
	github.com/asinrus/distributed_go_limiter/mongostore v0.0.0
	github.com/asinrus/distributed_go_limiter/pgstore v0.0.0
	github.com/asinrus/distributed_go_limiter/redisstore v0.0.0
	github.com/jackc/pgx/v5 v5.7.1
	github.com/redis/go-redis/v9 v9.6.1
	go.etcd.io/etcd/client/v3 v3.5.16
	go.mongodb.org/mongo-driver v1.17.1
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.17.6 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.etcd.io/etcd/api/v3 v3.5.16 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.16 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.17.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240318140521-94a12d6c2237 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237 // indirect
	google.golang.org/grpc v1.64.1 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)

replace (
	github.com/asinrus/distributed_go_limiter/core => ../core
	github.com/asinrus/distributed_go_limiter/etcdstore => ../etcdstore
	github.com/asinrus/distributed_go_limiter/mongostore => ../mongostore
	github.com/asinrus/distributed_go_limiter/pgstore => ../pgstore
	github.com/asinrus/distributed_go_limiter/redisstore => ../redisstore
)
