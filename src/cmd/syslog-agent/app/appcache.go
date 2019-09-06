package app

import (
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/golang/groupcache"
	"log"
	"net/http"
	"strings"
)

type AppCache struct {
	group    *groupcache.Group
	peers    []string
	certFile string
	keyFile  string
	caFile   string
	client   *cfclient.Client
	cache    map[string][]byte
	log      *log.Logger
}

func NewAppCache(cfg Config, logger *log.Logger) *AppCache {
	ccConfig := cfclient.Config{
		ApiAddress:        cfg.Capi.Endpoint,
		Username:          cfg.Capi.ClientID,
		Password:          cfg.Capi.ClientSecret,
		SkipSslValidation: true,
	}
	client, err := cfclient.NewClient(&ccConfig)
	if err != nil {
		logger.Panic("encountered an error while setting up the cf client: %v", err)
	}

	peerIPs := strings.Split(cfg.GroupCache.Peers, ",")
	var peers []string
	for _, peer := range peerIPs {
		peers = append(peers, peer+":8088")
	}

	ac := &AppCache{
		peers:    peers,
		certFile: cfg.GroupCache.CertFile,
		keyFile:  cfg.GroupCache.KeyFile,
		caFile:   cfg.GroupCache.CAFile,
		client:   client,
		cache:    make(map[string][]byte),
		log:      logger,
	}

	group := groupcache.NewGroup("appcache", 1024*64, groupcache.GetterFunc(ac.getter))
	ac.group = group

	return ac
}

func (a *AppCache) Start() {
	pool := groupcache.NewHTTPPool("127.0.0.1:8088")
	pool.Set(a.peers...)

	http.ListenAndServeTLS("127.0.0.1:8088", a.certFile, a.keyFile, nil)
}

func (a *AppCache) Get(guid string) string {
	var b []byte
	err := a.group.Get(nil, guid, groupcache.AllocatingByteSliceSink(&b))
	if err != nil {
		a.log.Println("failed to get group key:", err.Error())
		return "failed to get group key"
	}

	return string(b)
}

func (a *AppCache) getter(ctx groupcache.Context, key string, dest groupcache.Sink) error {
	v, ok := a.cache[key]
	if ok == true {
		dest.SetBytes(v)
		return nil
	}

	app, err := a.client.AppByGuid(key)
	if err != nil {
		a.log.Println("Failed to get app by guid", err.Error())
		return err
	}

	space, _ := app.Space()
	// Is this not overriding the value at this key?
	dest.SetBytes([]byte(space.Name))
	return nil
}
