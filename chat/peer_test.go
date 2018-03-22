package chat

import (
	"testing"
	"crypto/ecdsa"
	"net"
	"fmt"
	"time"
	"sync"
	"bytes"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/sudachen/misc/out"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
)

const nodesCount = 7
const mesgsCount = 30
const maxPeersPerNode = nodesCount/2+1
const ringSize = 1000

func TestPropagation(t *testing.T) {
	ns := setup(nodesCount, ringSize, maxPeersPerNode, t)
	out.Info.Printf("%d nodes started", len(ns))

	type S struct{Hash;string}
	hs := make([]S,mesgsCount*nodesCount)

	ns.checkpoint(0,t)

	out.Info.Print("SENDING MESSAGES")

	// sending unique messages via every node
	var wg sync.WaitGroup
	for _, n := range ns {
		wg.Add(1)
		go func(n *testNode) {
			for i := 0; i < mesgsCount; i++ {
				s := fmt.Sprintf("test message %d via node %d", i, n.no)
				h, err := n.send("", s)
				if err != nil {
					t.Fatal(err)
				}
				hs[n.no*mesgsCount+i] = S{h,s}
			}
			wg.Done()
		}(n)
	}
	wg.Wait()

	out.Info.Print("CHECKING PROPAGATION FOR MESSAGES")

	var count int

	// check all nodes received all test messages
CheckPropagation:
	for _, n := range ns {
	WaitingFor10secMax:
		for i := 0; i < 10; i++ { // 20 seconds maximum
			count = 0
			for _, v := range hs {
				if !n.has(v.Hash) {
					count++
				}
			}
			if count == 0 {
				break WaitingFor10secMax
			}
			out.Info.Printf("Waiting for node %d, it missed %d messages", n.no, count)
			<- time.After(time.Second)
		}
		if count != 0 {
			break CheckPropagation
		}
	}

	ns.stop()

	if count != 0 {
		var bf bytes.Buffer
		bf.WriteString("-\n")
		for _, n := range ns {
			count = 0
			for _, v := range hs {
				if !n.has(v.Hash) {
					count++
				}
			}
			bf.WriteString(fmt.Sprintf("node %d lost %d of %d messages\n", n.no, count, len(hs)))
			for _, v := range hs {
				if !n.has(v.Hash) {
					bf.WriteString(fmt.Sprintf("      %v: %v\n", v.Hash, v.string))
				}
			}
			iid := discover.PubkeyID(&n.id.PublicKey)
			bf.WriteString(fmt.Sprintf("ring (%v):\n",common.Bytes2Hex(iid[:8])))
			m, i := n.board.get(0)
			for m != nil {
				mesg, _ := m.open()
				peer := "self"
				if m.peer != nil {
					peer = common.Bytes2Hex(m.peer.ID()[:8])
				}
				bf.WriteString(fmt.Sprintf(" %3d, %v: %v, %v\n", i-1, m.hash(), mesg.Text, peer))
				m, i = n.board.get(i)
			}
		}
		t.Fatalf(bf.String())
	}
}

type testNode struct {
	no 		int
	id      *ecdsa.PrivateKey
	server  *p2p.Server
	board   *board
	quit    chan struct{}
	hashes  map[Hash]struct{}
	mu		sync.Mutex
}

type nodes []*testNode

func setup(nodesCount int, boardRingSize int, maxPeersPerNode int, t *testing.T) (ns nodes) {
	var err error
	ip := net.IPv4(127, 0, 0, 1)
	port0 := 30999

	if nodesCount > keysCount {
		t.Fatalf("to many nodes")
	}

	ns = make(nodes, 0, nodesCount)

	for i := 0; i < nodesCount; i++ {
		var node testNode

		node.no = i
		node.id, err = crypto.HexToECDSA(keys[i])
		if err != nil {
			t.Fatalf("failed convert the key: %s", keys[i])
		}

		port := port0 + i
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		name := common.MakeName("chat-go", "1.0")
		var peers []*discover.Node
		if i > 0 {
			peerNodeID := ns[i-1].id
			peerPort := uint16(port - 1)
			peerNode := discover.PubkeyID(&peerNodeID.PublicKey)
			peer := discover.NewNode(peerNode, ip, peerPort, peerPort)
			peers = append(peers, peer)
		}

		node.board = newBoard(boardRingSize)

		node.server = &p2p.Server{
			Config: p2p.Config{
				PrivateKey:     node.id,
				MaxPeers:       maxPeersPerNode,
				Name:           name,
				Protocols:      protocols(node.board,100000),
				ListenAddr:     addr,
				NAT:            nat.Any(),
				BootstrapNodes: peers,
				StaticNodes:    peers,
				TrustedNodes:   peers,
			},
		}

		ns = append(ns, &node)
	}

	ec := make(chan error,nodesCount)
	ns[0].start(ec)
	for i := 1; i < nodesCount; i++ {
		go ns[i].start(ec)
	}

	for i := 0; i < nodesCount; i++ {
		err := <- ec
		if err != nil {
			t.Fatal(err)
		}
	}

	return
}

func (n *testNode) has(h Hash) bool {
	n.mu.Lock()
	_, ok := n.hashes[h]
	n.mu.Unlock()
	return ok
}

func (n *testNode) watch() {
	delay := time.NewTicker(100*time.Millisecond)
	var index uint64
	var m *message
	for {
		if done2(n.quit, delay.C) {
			return
		}
		m, index = n.board.get(index)
		for m != nil {
			log.Debug("watch", "hash", m.hash())

			if mesg, err := m.open(); err != nil {
				log.Error("mesg open error", "m", m, "err", err)
			} else {
				n.mu.Lock()
				n.hashes[mesg.Hash()] = struct{}{}
				n.mu.Unlock()
			}

			if done(n.quit) {
				return
			}
			m, index = n.board.get(index)
		}
	}
}

func (ns nodes) checkpoint(no int, t *testing.T) {
	out.Info.Printf("CHECKPOINT %d", no)

	h, err := ns[0].send("", fmt.Sprintf("CHECKPOINT %d",no))
	if err != nil {
		t.Fatal(err)
	}

	// waiting until all nodes receive END message
	for j, n := range ns {
		succeeded := false
	WaitForEndMesg:
		for i := 0; i < 100; i++ { // 10 seconds max
			if _, ok := n.hashes[h]; ok {
				succeeded = true
				break WaitForEndMesg
			}
			<-time.After(100 * time.Millisecond)
		}
		if !succeeded {
			t.Fatalf("node %d has not recived checkpoint message", j)
		}
	}
}

func (n *testNode) start(ec chan error) {
	err := n.server.Start()
	if err != nil {
		ec <- fmt.Errorf("failed to start the server %d: %v", n.no, err)
		return
	}
	n.quit = make(chan struct{})
	n.hashes = make(map[Hash]struct{})
	go n.watch()
	go n.board.expire(n.quit)
	ec <- nil
}


func (n *testNode) stop() {
	n.server.Stop()
	close(n.quit)
}

func (ns nodes) stop() {
	for i := 0; i < len(ns); i++ {
		n := ns[i]
		if n != nil {
			n.stop()
		}
	}
}

func (n *testNode) send(room, text string) (Hash, error) {
	m := &message{}
	if err := m.seal(&Message{Room: room, Text: text, TTL: 10000}); err != nil {
		return Hash{}, err
	}
	n.board.put(m)
	return m.hash(), nil
}

const keysCount = 32

var keys = [keysCount]string{
	"d49dcf37238dc8a7aac57dc61b9fee68f0a97f062968978b9fafa7d1033d03a9",
	"73fd6143c48e80ed3c56ea159fe7494a0b6b393a392227b422f4c3e8f1b54f98",
	"119dd32adb1daa7a4c7bf77f847fb28730785aa92947edf42fdd997b54de40dc",
	"deeda8709dea935bb772248a3144dea449ffcc13e8e5a1fd4ef20ce4e9c87837",
	"5bd208a079633befa349441bdfdc4d85ba9bd56081525008380a63ac38a407cf",
	"1d27fb4912002d58a2a42a50c97edb05c1b3dffc665dbaa42df1fe8d3d95c9b5",
	"15def52800c9d6b8ca6f3066b7767a76afc7b611786c1276165fbc61636afb68",
	"51be6ab4b2dc89f251ff2ace10f3c1cc65d6855f3e083f91f6ff8efdfd28b48c",
	"ef1ef7441bf3c6419b162f05da6037474664f198b58db7315a6f4de52414b4a0",
	"09bdf6985aabc696dc1fbeb5381aebd7a6421727343872eb2fadfc6d82486fd9",
	"15d811bf2e01f99a224cdc91d0cf76cea08e8c67905c16fee9725c9be71185c4",
	"2f83e45cf1baaea779789f755b7da72d8857aeebff19362dd9af31d3c9d14620",
	"73f04e34ac6532b19c2aae8f8e52f38df1ac8f5cd10369f92325b9b0494b0590",
	"1e2e07b69e5025537fb73770f483dc8d64f84ae3403775ef61cd36e3faf162c1",
	"8963d9bbb3911aac6d30388c786756b1c423c4fbbc95d1f96ddbddf39809e43a",
	"0422da85abc48249270b45d8de38a4cc3c02032ede1fcf0864a51092d58a2f1f",
	"8ae5c15b0e8c7cade201fdc149831aa9b11ff626a7ffd27188886cc108ad0fa8",
	"acd8f5a71d4aecfcb9ad00d32aa4bcf2a602939b6a9dd071bab443154184f805",
	"a285a922125a7481600782ad69debfbcdb0316c1e97c267aff29ef50001ec045",
	"28fd4eee78c6cd4bf78f39f8ab30c32c67c24a6223baa40e6f9c9a0e1de7cef5",
	"c5cca0c9e6f043b288c6f1aef448ab59132dab3e453671af5d0752961f013fc7",
	"46df99b051838cb6f8d1b73f232af516886bd8c4d0ee07af9a0a033c391380fd",
	"c6a06a53cbaadbb432884f36155c8f3244e244881b5ee3e92e974cfa166d793f",
	"783b90c75c63dc72e2f8d11b6f1b4de54d63825330ec76ee8db34f06b38ea211",
	"9450038f10ca2c097a8013e5121b36b422b95b04892232f930a29292d9935611",
	"e215e6246ed1cfdcf7310d4d8cdbe370f0d6a8371e4eb1089e2ae05c0e1bc10f",
	"487110939ed9d64ebbc1f300adeab358bc58875faf4ca64990fbd7fe03b78f2b",
	"824a70ea76ac81366da1d4f4ac39de851c8ac49dca456bb3f0a186ceefa269a5",
	"ba8f34fa40945560d1006a328fe70c42e35cc3d1017e72d26864cd0d1b150f15",
	"30a5dfcfd144997f428901ea88a43c8d176b19c79dde54cc58eea001aa3d246c",
	"de59f7183aca39aa245ce66a05245fecfc7e2c75884184b52b27734a4a58efa2",
	"92629e2ff5f0cb4f5f08fffe0f64492024d36f045b901efb271674b801095c5a",
}
