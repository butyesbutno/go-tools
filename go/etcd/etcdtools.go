package etcdtools

import (
	"context"
	"strings"
	"time"
	"math/rand"

	"errors"
	"io/ioutil"
	"crypto/tls"
	"crypto/x509"	

	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/butyesbutno/tools/go/log"
)

const (
	// OpTimeout operator timeout
	OpTimeout = 5
)
var (
	chanExitCmd = make(chan int)
	etcdClient *clientv3.Client
	etcdConfig = clientv3.Config{
		Endpoints:   []string{"localhost:2379"}, //"localhost:2379"},
		DialTimeout: 5 * time.Second,
		// Transport: client.DefaultTransport,
		// Username:  etcdUsername,
		// Password:  etcdPassword,
	}
)

// GetEtcdConfig current etcd config
func GetEtcdConfig() *clientv3.Config {
	return &etcdConfig
}

// SetEtcdConfig current etcd config
func SetEtcdConfig(config *clientv3.Config) {
	etcdConfig = *config
	closeConnect()
}

// SetTLSConfig set tls config
func SetTLSConfig(etcdCert, etcdCertKey, etcdCa string, endpoints []string) error {
	cert, err := tls.LoadX509KeyPair(etcdCert, etcdCertKey)
	if err != nil {
		return errors.New("tls failed")
	}

	caData, err := ioutil.ReadFile(etcdCa)
	if err != nil {
		return errors.New("tls failed read file")
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caData)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}

	etcdConfig = clientv3.Config{
		Endpoints: endpoints,
		DialTimeout: 5 * time.Second,
		TLS:       tlsConfig,
	}
	closeConnect()
	return nil
}

// close connection to etcd endpoint
func closeConnect() {
	if etcdClient == nil {
		return
	}
	etcdClient.Close()
	etcdClient = nil
}

// setup connection to etcd endpoint
func setupConnect( ) error {
	if etcdClient != nil {
		return nil
	}
	client, err := clientv3.New(etcdConfig)
	if err != nil {
		return err
	}
	etcdClient = client
	return nil
}

// GetKey get value with prefix
func GetKey(prefix string) ([]string, error) {

	if err := setupConnect(); err != nil {
		return nil, err
	}

	// Get operator
	kv := clientv3.NewKV(etcdClient)
	ctx, _ := context.WithTimeout(context.TODO(), OpTimeout * time.Second)
	rangeResp, err := kv.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		closeConnect()
		return nil, err
	}

	// all value with prefix
	list := []string{}
	for _, kv := range rangeResp.Kvs {
		list = append(list, string(kv.Value))
	}
	return list, nil
}

// SetKey key value, ttl<=0 means exist forever. otherwise your need refresh manually
func SetKey(key, value string, secondsTTL int) error {

	if err := setupConnect(); err != nil {
		return err
	}

	kv := clientv3.NewKV(etcdClient)
	ctx, _ := context.WithTimeout(context.TODO(), OpTimeout * time.Second)
	if secondsTTL > 0 {
		lease := clientv3.NewLease(client)
		leaseResp, lerr := lease.Grant(ctx, int64(secondsTTL))
		if lerr != nil {
			closeConnect()
			return lerr
		}

		ctx, _ = context.WithTimeout(context.TODO(), OpTimeout * time.Second)
		if _, perr := kv.Put(ctx, key, value, clientv3.WithLease(leaseResp.ID)); perr != nil {
			closeConnect()
			return perr
		}
	} else {
		if _, err := kv.Put(ctx, key, value, clientv3.WithPrevKV()); err != nil {
			closeConnect()
			return err
		}
	}
	return nil
}

// StopETCDRegister stop
func StopETCDRegister() {
	chanExitCmd <- 1
	<-chanExitCmd
}

// GetServiceRoundRobin get service by round-robin
func GetServiceRoundRobin(prefix string) string {
	list, err := GetKey(prefix)
	if err != nil {
		return ""
	}

	listLen := len(list)
	if listLen < 1 {
		return ""
	}
	if listLen == 1 {
		return list[0]
	}

	rand.Seed(time.Now().UnixNano())
	l := listLen
	if l < 100 {
		l = 100
	}
	r := rand.Intn(l)
	if r >= listLen {
		r = r % listLen
	}

	return list[r]
}

// PutService register the service
func PutService(key, value string, ttl int) {
	go putServiceImpl(key, value, ttl)
}

// putServiceImpl
func putServiceImpl(key, value string, ttl int) {
	key = strings.TrimRight(key, "/") + "/"
	
	for {
		select {
		case <-chanExitCmd:
			chanExitCmd <- 1
			return
		default:
		}

		client, err := clientv3.New(etcdConfig)
		if err != nil {
			time.Sleep(time.Duration(1) * time.Second)
			continue
		}

		kv := clientv3.NewKV(client)
		lease := clientv3.NewLease(client)
		var theLeaseID clientv3.LeaseID
		theLeaseID = 0

		lastLeaseTime := time.Now()
		for {
			select {
			case <-chanExitCmd:
				chanExitCmd <- 1
				return
			default:
			}

			if theLeaseID == 0 {
				ctx, _ := context.WithTimeout(context.TODO(), OpTimeout * time.Second)
				leaseResp, err := lease.Grant(ctx, int64(ttl))
				if err != nil {
					client.Close()
					break
				}

				// key := key + fmt.Sprintf("%d", leaseResp.ID)
				ctx, _ = context.WithTimeout(context.TODO(), OpTimeout * time.Second)
				if _, err := kv.Put(ctx, key, value, clientv3.WithLease(leaseResp.ID)); err != nil {
					client.Close()
					break
				}
				theLeaseID = leaseResp.ID
				lastLeaseTime = time.Now()
				commonLog.LogInfo("PutService: " + key + "<->" + value)
			} else {
				tm := time.Now()
				expectT := lastLeaseTime.Add(time.Duration( (ttl * 1000) - 500) * time.Millisecond)
				if tm.Before(expectT) == false {
					// 续约租约，如果租约已经过期将theLeaseID复位到0重新走创建租约的逻辑
					ctx, _ := context.WithTimeout(context.TODO(), OpTimeout * time.Second)
					if _, err := lease.KeepAliveOnce(ctx, theLeaseID); err == rpctypes.ErrLeaseNotFound {
						theLeaseID = 0
						continue
					} 
					lastLeaseTime = tm
				}
			}
			time.Sleep(time.Duration(500) * time.Millisecond)
		}
	}
}