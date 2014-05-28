package go4game

import (
	//"fmt"
	"log"
	//"math"
	//"math/rand"
	//"net"
	"bytes"
	//"reflect"
	htemplate "html/template"
	"net/http"
	"sort"
	ttemplate "text/template"
	//"time"
	"github.com/gorilla/websocket"
	"strconv"
)

type TeamInfo struct {
	ID         int64
	ClientInfo string
	Objs       int
	AP         int
	PacketStat string
	CollStat   string
	Color      int
	FontColor  int
	Score      int
}

func (t *Team) NewTeamInfo() *TeamInfo {
	return &TeamInfo{
		ID:         t.ID,
		ClientInfo: t.ClientConnInfo.String(),
		Objs:       len(t.GameObjs),
		AP:         t.ActionPoint,
		PacketStat: t.PacketStat.String(),
		CollStat:   t.CollisionStat.String(),
		Color:      t.Color,
		FontColor:  0xffffff ^ t.Color,
		Score:      t.Score,
	}
}

type ByScore []TeamInfo

func (s ByScore) Len() int {
	return len(s)
}
func (s ByScore) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s ByScore) Less(i, j int) bool {
	return s[i].Score > s[j].Score
}

var WorldTextTemplate *ttemplate.Template

type WorldInfo struct {
	Disp  string
	Teams []TeamInfo
}

func (m *World) makeWorldInfo() *WorldInfo {
	rtn := &WorldInfo{
		Disp:  m.String(),
		Teams: make([]TeamInfo, 0, len(m.Teams)),
	}
	for _, t := range m.Teams {
		rtn.Teams = append(rtn.Teams, *t.NewTeamInfo())
	}
	sort.Sort(ByScore(rtn.Teams))
	return rtn
}

func (wi WorldInfo) String() string {
	var w bytes.Buffer
	WorldTextTemplate.Execute(&w, wi)
	return w.String()
}

var TopHtmlTemplate *htemplate.Template
var WorldHtmlTemplate *htemplate.Template

func init() {
	const ttworld = `
{{.Disp}}
TeamColor TeamID ClientInfo ObjCount Score ActionPoint PacketStat CollStat {{range .Teams}}
{{.FontColor | printf "%x"}} {{.ID}} {{.ClientInfo}} {{.Objs}} {{.Score}} {{.AP}} {{.PacketStat}} {{.CollStat}} {{end}}
`
	WorldTextTemplate = ttemplate.Must(ttemplate.New("indexpage").Parse(ttworld))

	const tindex = `
        <html>
        <head>
        <title>go4game stat</title>
        <meta http-equiv="refresh" content="1">
        </head>
        <body>
        <a href='www/client3d.html' target="_blank">Open 3d client</a>
        </br>
        {{.Disp}}
        </br>
        {{range $id, $s := .Worlds}}
        <a href='?worldid={{$id}}' target="_blank">{{$s}}</a>
        </br>
        {{end}}
        </body>
        </html>
        `
	TopHtmlTemplate = htemplate.Must(htemplate.New("indexpage").Parse(tindex))

	const tworld = `
        <html>
        <head>
        <title>go4game stat</title>
        <meta http-equiv="refresh" content="1">
        </head>
        <body>
        <a href='www/client3d.html' target="_blank">Open 3d client</a>
        </br>
        {{.Disp}}
        </br>
        <table>
        <tr >
            <td>TeamID</td>
            <td>ClientInfo</td>
            <td>ObjCount</td>
            <td>Score</td>
            <td>ActionPoint</td>
            <td>PacketStat</td>
            <td>CollStat</td>
        </tr>
        {{range .Teams}}
        <tr bgcolor="#{{.Color | printf "%x"}}">
            <td><font color="#{{.FontColor | printf "%x"}}">{{.ID}}</font></td>
            <td><font color="#{{.FontColor | printf "%x"}}">{{.ClientInfo}}</font></td>
            <td><font color="#{{.FontColor | printf "%x"}}">{{.Objs}}</font></td>
            <td><font color="#{{.FontColor | printf "%x"}}">{{.Score}}</font></td>
            <td><font color="#{{.FontColor | printf "%x"}}">{{.AP}}</font></td>
            <td><font color="#{{.FontColor | printf "%x"}}">{{.PacketStat}}</font></td>
            <td><font color="#{{.FontColor | printf "%x"}}">{{.CollStat}}</font></td>
        </tr>
        {{end}}
        </table>
        </body>
        </html>
        `
	WorldHtmlTemplate = htemplate.Must(htemplate.New("indexpage").Parse(tworld))
}

// web socket server
func (g *GameService) wsServer() {
	http.HandleFunc("/ws", g.wsServe)
	http.HandleFunc("/", g.Stat)
	http.Handle("/www/", http.StripPrefix("/www/", http.FileServer(http.Dir("./www"))))
	err := http.ListenAndServe(GameConst.WsListen, nil)
	if err != nil {
		log.Println("ListenAndServe: ", err)
	}
}

func (g *GameService) Stat(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "parse error", 405)
	}
	wid := r.Form.Get("worldid")
	worldid, err := strconv.ParseInt(wid, 0, 64)
	//log.Printf("worldid %v, %v", worldid, err)

	if err != nil {
		ws := make(map[int64]string, len(g.Worlds))
		for id, w := range g.Worlds {
			ws[id] = w.String()
		}
		TopHtmlTemplate.Execute(w, struct {
			Disp   string
			Worlds map[int64]string
		}{
			Disp:   g.String(),
			Worlds: ws,
		})
	} else {
		if g.Worlds[worldid] != nil {
			wi := g.Worlds[worldid].makeWorldInfo()
			WorldHtmlTemplate.Execute(w, wi)
		}
	}
}

func (g *GameService) wsServe(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if _, ok := err.(websocket.HandshakeError); !ok {
			log.Println(err)
		}
		return
	}
	g.wsClientConnectionCh <- ws
}
