package agent

import (
	"github.com/hashicorp/serf/testutil"
	"net"
	"net/rpc"
	"strings"
	"testing"
)

// testRPCClient returns an RPCClient connected to an RPC server that
// serves only this connection.
func testRPCClient(t *testing.T) (*RPCClient, *Agent) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	agent := testAgent()
	server := rpc.NewServer()
	if err := registerEndpoint(server, agent); err != nil {
		l.Close()
		t.Fatalf("err: %s", err)
	}

	go func() {
		conn, err := l.Accept()
		l.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		defer conn.Close()
		server.ServeConn(conn)
	}()

	rpcClient, err := rpc.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	return &RPCClient{Client: rpcClient}, agent
}

func TestRPCClientJoin(t *testing.T) {
	client, a1 := testRPCClient(t)
	a2 := testAgent()
	defer client.Close()
	defer a1.Shutdown()
	defer a2.Shutdown()

	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	if err := a2.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	n, err := client.Join([]string{a2.SerfConfig.MemberlistConfig.BindAddr})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if n != 1 {
		t.Fatalf("n != 1: %d", n)
	}
}

func TestRPCClientMembers(t *testing.T) {
	client, a1 := testRPCClient(t)
	a2 := testAgent()
	defer client.Close()
	defer a1.Shutdown()
	defer a2.Shutdown()

	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	if err := a2.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	mem, err := client.Members()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(mem) != 1 {
		t.Fatalf("bad: %#v", mem)
	}

	_, err = client.Join([]string{a2.SerfConfig.MemberlistConfig.BindAddr})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	mem, err = client.Members()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(mem) != 2 {
		t.Fatalf("bad: %#v", mem)
	}
}

func TestRPCClientMonitor(t *testing.T) {
	client, a1 := testRPCClient(t)
	defer client.Close()
	defer a1.Shutdown()

	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	eventCh := make(chan string, 64)
	doneCh := make(chan struct{}, 64)
	defer close(doneCh)
	if err := client.Monitor(eventCh, doneCh); err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	select {
	case e := <-eventCh:
		if !strings.Contains(e, "starting") {
			t.Fatalf("bad: %s", e)
		}
	default:
		t.Fatalf("should have backlog")
	}

	// Drain the rest of the messages as we know it
	drainEventCh(eventCh)

	// Join a bad thing to generate more events
	a1.Join(nil)

	testutil.Yield()

	select {
	case e := <-eventCh:
		if !strings.Contains(e, "joining") {
			t.Fatalf("bad: %s", e)
		}
	default:
		t.Fatalf("should have message")
	}

	// End the monitor
	doneCh <- struct{}{}
	testutil.Yield()
	drainEventCh(eventCh)

	// Do another thing to generate more events
	a1.Join(nil)

	testutil.Yield()

	select {
	case e := <-eventCh:
		t.Fatalf("should have no more: %s", e)
	default:
	}
}
