package etcdtools

import (
	"context"
	"strings"
	"time"
	"math/rand"

	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/butyesbutno/go-tools/log"
)

const (
	// OpTimeout operator timeout
	OpTimeout = 5
)
var (
	chanExitCmd = make(chan int)
	etcdClient *clientv3.Client
)

func setupConnect(etcdAddress string) error {
	if etcdClient != nil {
		return nil
	}
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{etcdAddress}, //"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return err
	}
	etcdClient = client
	return nil
}

// GetKey get value with prefix
func GetKey(etcdAddress, prefix string) ([]string, error) {

	if err := setupConnect(etcdAddress); err != nil {
		return nil, err
	}

	// Get operator
	kv := clientv3.NewKV(etcdClient)
	ctx, _ := context.WithTimeout(context.TODO(), OpTimeout * time.Second)
	rangeResp, err := kv.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		etcdClient.Close()
		etcdClient = nil
		return nil, err
	}

	// all value with prefix
	list := []string{}
	for _, kv := range rangeResp.Kvs {
		list = append(list, string(kv.Value))
	}
	return list, nil
}

// SetKey key value exist forever
func SetKey(etcdAddress, key, value string) error {

	if err := setupConnect(etcdAddress); err != nil {
		return err
	}

	kv := clientv3.NewKV(etcdClient)
	ctx, _ := context.WithTimeout(context.TODO(), OpTimeout * time.Second)
	if _, err := kv.Put(ctx, key, value, clientv3.WithPrevKV()); err != nil {
		etcdClient.Close()
		etcdClient = nil
		return err
	}
	return nil
}

// StopETCDRegister stop
func StopETCDRegister() {
	chanExitCmd <- 1
	<-chanExitCmd
}

// GetServiceRoundRobin get service by round-robin
func GetServiceRoundRobin(etcdAddress, prefix string) string {
	list, err := GetKey(etcdAddress, prefix)
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
func PutService(etcdAddress, key, value string, ttl int) {
	go putServiceImpl(etcdAddress, key, value, ttl)
}

// putServiceImpl
func putServiceImpl(etcdAddress, key, value string, ttl int) {
	key = strings.TrimRight(key, "/") + "/"
	
	for {
		select {
		case <-chanExitCmd:
			chanExitCmd <- 1
			return
		default:
		}

		client, err := clientv3.New(clientv3.Config{
			Endpoints:   []string{etcdAddress}, //"localhost:2379"},
			DialTimeout: 5 * time.Second,
		})
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