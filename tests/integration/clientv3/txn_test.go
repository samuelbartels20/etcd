// Copyright 2016 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package clientv3test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	integration2 "go.etcd.io/etcd/tests/v3/framework/integration"
)

func TestTxnError(t *testing.T) {
	integration2.BeforeTest(t)

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 1})
	defer clus.Terminate(t)

	kv := clus.RandClient()
	ctx := t.Context()

	_, err := kv.Txn(ctx).Then(clientv3.OpPut("foo", "bar1"), clientv3.OpPut("foo", "bar2")).Commit()
	if !errors.Is(err, rpctypes.ErrDuplicateKey) {
		t.Fatalf("expected %v, got %v", rpctypes.ErrDuplicateKey, err)
	}

	ops := make([]clientv3.Op, int(embed.DefaultMaxTxnOps+10))
	for i := range ops {
		ops[i] = clientv3.OpPut(fmt.Sprintf("foo%d", i), "")
	}
	_, err = kv.Txn(ctx).Then(ops...).Commit()
	if !errors.Is(err, rpctypes.ErrTooManyOps) {
		t.Fatalf("expected %v, got %v", rpctypes.ErrTooManyOps, err)
	}
}

func TestTxnWriteFail(t *testing.T) {
	integration2.BeforeTest(t)

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 3, UseBridge: true})
	defer clus.Terminate(t)

	kv := clus.Client(0)

	clus.Members[0].Stop(t)

	txnc, getc := make(chan struct{}), make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		resp, err := kv.Txn(ctx).Then(clientv3.OpPut("foo", "bar")).Commit()
		if err == nil {
			t.Errorf("expected error, got response %v", resp)
		}
		close(txnc)
	}()

	go func() {
		defer close(getc)
		select {
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for txn fail")
		case <-txnc:
		}
		// and ensure the put didn't take
		gresp, gerr := clus.Client(1).Get(t.Context(), "foo")
		if gerr != nil {
			t.Error(gerr)
		}
		if len(gresp.Kvs) != 0 {
			t.Errorf("expected no keys, got %v", gresp.Kvs)
		}
	}()

	select {
	case <-time.After(5 * clus.Members[1].ServerConfig.ReqTimeout()):
		t.Fatalf("timed out waiting for get")
	case <-getc:
	}

	// reconnect so terminate doesn't complain about double-close
	clus.Members[0].Restart(t)
}

func TestTxnReadRetry(t *testing.T) {
	t.Skipf("skipping txn read retry test: re-enable after we do retry on txn read request")

	integration2.BeforeTest(t)

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 3, UseBridge: true})
	defer clus.Terminate(t)

	kv := clus.Client(0)

	thenOps := [][]clientv3.Op{
		{clientv3.OpGet("foo")},
		{clientv3.OpTxn(nil, []clientv3.Op{clientv3.OpGet("foo")}, nil)},
		{clientv3.OpTxn(nil, nil, nil)},
		{},
	}
	for i := range thenOps {
		clus.Members[0].Stop(t)
		<-clus.Members[0].StopNotify()

		donec := make(chan struct{}, 1)
		go func() {
			_, err := kv.Txn(t.Context()).Then(thenOps[i]...).Commit()
			if err != nil {
				t.Errorf("expected response, got error %v", err)
			}
			donec <- struct{}{}
		}()
		// wait for txn to fail on disconnect
		time.Sleep(100 * time.Millisecond)

		// restart node; client should resume
		clus.Members[0].Restart(t)
		select {
		case <-donec:
		case <-time.After(2 * clus.Members[1].ServerConfig.ReqTimeout()):
			t.Fatalf("waited too long")
		}
	}
}

func TestTxnSuccess(t *testing.T) {
	integration2.BeforeTest(t)

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 3})
	defer clus.Terminate(t)

	kv := clus.Client(0)
	ctx := t.Context()

	_, err := kv.Txn(ctx).Then(clientv3.OpPut("foo", "bar")).Commit()
	require.NoError(t, err)

	resp, err := kv.Get(ctx, "foo")
	require.NoError(t, err)
	if len(resp.Kvs) != 1 || string(resp.Kvs[0].Key) != "foo" {
		t.Fatalf("unexpected Get response %v", resp)
	}
}

func TestTxnCompareRange(t *testing.T) {
	integration2.BeforeTest(t)

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 1})
	defer clus.Terminate(t)

	kv := clus.Client(0)
	fooResp, err := kv.Put(t.Context(), "foo/", "bar")
	require.NoError(t, err)
	_, err = kv.Put(t.Context(), "foo/a", "baz")
	require.NoError(t, err)
	tresp, terr := kv.Txn(t.Context()).If(
		clientv3.Compare(
			clientv3.CreateRevision("foo/"), "=", fooResp.Header.Revision).
			WithPrefix(),
	).Commit()
	require.NoError(t, terr)
	if tresp.Succeeded {
		t.Fatal("expected prefix compare to false, got compares as true")
	}
}

func TestTxnNested(t *testing.T) {
	integration2.BeforeTest(t)

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 3})
	defer clus.Terminate(t)

	kv := clus.Client(0)

	tresp, err := kv.Txn(t.Context()).
		If(clientv3.Compare(clientv3.Version("foo"), "=", 0)).
		Then(
			clientv3.OpPut("foo", "bar"),
			clientv3.OpTxn(nil, []clientv3.Op{clientv3.OpPut("abc", "123")}, nil)).
		Else(clientv3.OpPut("foo", "baz")).Commit()
	require.NoError(t, err)
	if len(tresp.Responses) != 2 {
		t.Errorf("expected 2 top-level txn responses, got %+v", tresp.Responses)
	}

	// check txn writes were applied
	resp, err := kv.Get(t.Context(), "foo")
	require.NoError(t, err)
	if len(resp.Kvs) != 1 || string(resp.Kvs[0].Value) != "bar" {
		t.Errorf("unexpected Get response %+v", resp)
	}
	resp, err = kv.Get(t.Context(), "abc")
	require.NoError(t, err)
	if len(resp.Kvs) != 1 || string(resp.Kvs[0].Value) != "123" {
		t.Errorf("unexpected Get response %+v", resp)
	}
}
