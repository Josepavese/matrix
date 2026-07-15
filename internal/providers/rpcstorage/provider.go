package rpcstorage

import (
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
)

const dialTimeout = 750 * time.Millisecond

type Provider struct {
	client *rpc.Client
	token  string
}

func New(addr, token string) (*Provider, error) {
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect runtime vault broker at %s: %w", addr, err)
	}
	return &Provider{client: rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn)), token: token}, nil
}

func (p *Provider) Get(key string) ([]byte, error) {
	reply := runtimebroker.StorageReply{}
	err := p.call("Storage.Get", runtimebroker.StorageArgs{Key: key}, &reply)
	return reply.Value, err
}

func (p *Provider) Set(key string, value []byte) error {
	return p.call("Storage.Set", runtimebroker.StorageArgs{Key: key, Value: value}, &runtimebroker.StorageReply{})
}

func (p *Provider) Delete(key string) error {
	return p.call("Storage.Delete", runtimebroker.StorageArgs{Key: key}, &runtimebroker.StorageReply{})
}

func (p *Provider) List(prefix string) ([]string, error) {
	reply := runtimebroker.StorageReply{}
	err := p.call("Storage.List", runtimebroker.StorageArgs{Prefix: prefix}, &reply)
	return reply.Keys, err
}

func (p *Provider) Close() error {
	return p.client.Close()
}

func (p *Provider) call(method string, args runtimebroker.StorageArgs, reply *runtimebroker.StorageReply) error {
	args.Token = p.token
	if err := p.client.Call(method, &args, reply); err != nil {
		return fmt.Errorf("runtime vault broker %s: %w", method, err)
	}
	return nil
}
