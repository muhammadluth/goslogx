package redis

import (
	"runtime"
	"sync"
	"weak"

	goredis "github.com/redis/go-redis/v9"
)

// Weak identities prevent the registry from retaining clients or confusing a
// later allocation at the same address with an already instrumented client.
var registrations = struct {
	sync.Mutex
	clients map[any]*registration
}{clients: make(map[any]*registration)}

type registration struct {
	done chan struct{}
	err  error
}

func register[T any](client *T) (*registration, bool) {
	key := weak.Make(client)
	registrations.Lock()
	defer registrations.Unlock()
	if existing := registrations.clients[key]; existing != nil {
		return existing, false
	}
	r := &registration{done: make(chan struct{})}
	registrations.clients[key] = r
	runtime.AddCleanup(client, func(key weak.Pointer[T]) {
		registrations.Lock()
		delete(registrations.clients, key)
		registrations.Unlock()
	}, key)
	return r, true
}

func registerClient(client goredis.UniversalClient) (*registration, bool) {
	switch c := client.(type) {
	case *goredis.Client:
		return register(c)
	case *goredis.ClusterClient:
		return register(c)
	case *goredis.Ring:
		return register(c)
	default:
		panic("unsupported client passed validation")
	}
}
