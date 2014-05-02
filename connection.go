package go4game

import (
	//"encoding/binary"
	"encoding/json"
	//"errors"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"time"
)

const (
	writeWait      = 10 * time.Second    // Time allowed to write a message to the peer.
	pongWait       = 60 * time.Second    // Time allowed to read the next pong message from the peer.
	pingPeriod     = (pongWait * 9) / 10 // Send pings to peer with this period. Must be less than pongWait.
	maxMessageSize = 0xffff              // Maximum message size allowed from peer.
)

const (
	_ = iota
	TCPClient
	WebSockClient
	AIClient
)

type ClientType int

type ConnInfo struct {
	Stat       *PacketStat
	PTeam      *Team
	ReadCh     chan *GamePacket
	WriteCh    chan *GamePacket
	clientType ClientType
	Conn       net.Conn
	WsConn     *websocket.Conn
	AiConn     *AIConn
}

type AIConn struct {
	pteam *Team
}

func NewAIConnInfo(t *Team, aiconn *AIConn) *ConnInfo {
	c := ConnInfo{
		Stat:       NewPacketStatInfo(),
		ReadCh:     make(chan *GamePacket, 1),
		WriteCh:    make(chan *GamePacket, 1),
		PTeam:      t,
		AiConn:     aiconn,
		clientType: AIClient,
	}
	aiconn.pteam = t
	go c.aiLoop()
	return &c
}

func (c *ConnInfo) aiLoop() {
	defer func() {
		//log.Printf("aiLoop end team:%v", c.PTeam.ID)
		close(c.ReadCh)
	}()
	//timer60Ch := time.Tick(1000 / 60 * time.Millisecond)
	var worldinfo *WorldSerialize
	c.ReadCh <- &GamePacket{
		Cmd: ReqWorldInfo,
	}
loop:
	for {
		select {
		case packet, ok := <-c.WriteCh: // get rsp from server
			if !ok {
				break loop
			}
			c.Stat.IncW()
			switch packet.Cmd {
			case RspAIAct:
				worldinfo = nil
			case RspWorldInfo:
				worldinfo = packet.WorldInfo
			default:
				log.Printf("unknown packet %v", packet.Cmd)
				break loop
			}

			//case <-timer60Ch:
			if worldinfo == nil {
				c.ReadCh <- &GamePacket{
					Cmd: ReqWorldInfo,
				}
			} else {
				c.ReadCh <- c.AiConn.makeAIAction(worldinfo)
			}

			c.Stat.IncR()
		}
	}
}

func (a *AIConn) makeAIAction(worldinfo *WorldSerialize) *GamePacket {
	return &GamePacket{
		Cmd: ReqAIAct,
		ClientAct: &ClientActionPacket{
			Accel:          RandVector3D(-100, 100),
			NormalBulletMv: RandVector3D(-100, 100),
		},
	}
}

func NewTcpConnInfo(t *Team, conn net.Conn) *ConnInfo {
	c := ConnInfo{
		Stat:       NewPacketStatInfo(),
		Conn:       conn,
		ReadCh:     make(chan *GamePacket, 1),
		WriteCh:    make(chan *GamePacket, 1),
		PTeam:      t,
		clientType: TCPClient,
	}
	go c.tcpReadLoop()
	go c.tcpWriteLoop()
	return &c
}

func (c *ConnInfo) tcpReadLoop() {
	defer func() {
		c.Conn.Close()
		close(c.ReadCh)
		//log.Printf("tcpReadLoop end team:%v", c.PTeam.ID)
	}()
	dec := json.NewDecoder(c.Conn)
	for {
		var v GamePacket
		err := dec.Decode(&v)
		if err != nil {
			break
		}
		c.ReadCh <- &v
		c.Stat.IncR()
	}
}

func (c *ConnInfo) tcpWriteLoop() {
	defer func() {
		c.Conn.Close()
		//log.Printf("tcpWriteLoop end team:%v", c.PTeam.ID)
	}()
	enc := json.NewEncoder(c.Conn)
loop:
	for {
		select {
		case packet, ok := <-c.WriteCh:
			if !ok {
				break loop
			}
			err := enc.Encode(packet)
			if err != nil {
				break loop
			}
			c.Stat.IncW()
		}
	}
}

func NewWsConnInfo(t *Team, conn *websocket.Conn) *ConnInfo {
	c := ConnInfo{
		Stat:       NewPacketStatInfo(),
		WsConn:     conn,
		ReadCh:     make(chan *GamePacket, 1),
		WriteCh:    make(chan *GamePacket, 1),
		PTeam:      t,
		clientType: WebSockClient,
	}
	go c.wsReadLoop()
	go c.wsWriteLoop()
	return &c
}

func (c *ConnInfo) wsReadLoop() {
	defer func() {
		c.WsConn.Close()
		close(c.ReadCh)
		//log.Printf("wsReadLoop end team:%v", c.PTeam.ID)
	}()
	c.WsConn.SetReadLimit(maxMessageSize)
	c.WsConn.SetReadDeadline(time.Now().Add(pongWait))
	c.WsConn.SetPongHandler(func(string) error {
		c.WsConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		var v GamePacket
		err := c.WsConn.ReadJSON(&v)
		if err != nil {
			break
		}
		c.ReadCh <- &v
		c.Stat.IncR()
	}
}

func (c *ConnInfo) write(mt int, payload []byte) error {
	c.WsConn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.WsConn.WriteMessage(mt, payload)
}

func (c *ConnInfo) wsWriteLoop() {
	timerPing := time.Tick(pingPeriod)
	defer func() {
		c.WsConn.Close()
		//log.Printf("wsWriteLoop end team:%v", c.PTeam.ID)
	}()
	for {
		select {
		case packet, ok := <-c.WriteCh:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			message, err := json.Marshal(&packet)
			if err != nil {
				return
			}
			if err := c.write(websocket.TextMessage, message); err != nil {
				return
			}
			c.Stat.IncW()
		case <-timerPing:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}