package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	mediaserver "github.com/notedit/media-server-go"
	"github.com/notedit/sdp"
	"github.com/sanity-io/litter"
	"go.uber.org/zap"
)

type Message struct {
	Cmd      string `json:"cmd,omitempty"`
	Sdp      string `json:"sdp,omitempty"`
	StreamID string `json:"stream,omitempty"`
}

var upGrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var Capabilities = map[string]*sdp.Capability{
	"audio": &sdp.Capability{
		Codecs: []string{"opus"},
	},
	"video": &sdp.Capability{
		Codecs: []string{"vp8"},
		Rtx:    true,
		Rtcpfbs: []*sdp.RtcpFeedback{
			&sdp.RtcpFeedback{
				ID: "goog-remb",
			},
			&sdp.RtcpFeedback{
				ID: "transport-cc",
			},
			&sdp.RtcpFeedback{
				ID:     "ccm",
				Params: []string{"fir"},
			},
			&sdp.RtcpFeedback{
				ID:     "nack",
				Params: []string{"pli"},
			},
		},
		Extensions: []string{
			"urn:3gpp:video-orientation",
			"http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01",
			"http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time",
			"urn:ietf:params:rtp-hdrext:toffse",
			"urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id",
			"urn:ietf:params:rtp-hdrext:sdes:mid",
		},
	},
}

var incomingStreams = map[string]*mediaserver.IncomingStream{}

func channel(c *gin.Context) {

	ws, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	var transport *mediaserver.Transport
	endpoint := mediaserver.NewEndpoint("tizi.sunbin123.xyz")

	for {
		// read json
		var msg Message
		err = ws.ReadJSON(&msg)
		if err != nil {
			fmt.Println("error: ", err)
			break
		}

		if msg.Cmd == "publish" {
			offer, err := sdp.Parse(msg.Sdp)
			if err != nil {
				panic(err)
			}
			logger.Info("publish-offer", zap.Any("offer", offer))
			transport = endpoint.CreateTransport(offer, nil)
			transport.SetRemoteProperties(offer.GetMedia("audio"), offer.GetMedia("video"))

			answer := offer.Answer(
				transport.GetLocalICEInfo(),
				transport.GetLocalDTLSInfo(), //DTLS 是指 Datagram Transport Level Security，即数据报安全传输协议； 其提供了UDP 传输场景下的安全解决方案，能防止消息被窃听、篡改、身份冒充等问题。
				endpoint.GetLocalCandidates(),
				Capabilities)

			transport.SetLocalProperties(answer.GetMedia("audio"), answer.GetMedia("video"))

			for _, stream := range offer.GetStreams() {
				fmt.Println("stream-GetID", stream.GetID())
				incomingStream := transport.CreateIncomingStream(stream)
				incomingStreams[incomingStream.GetID()] = incomingStream
			}

			ws.WriteJSON(Message{
				Cmd: "answer",
				Sdp: answer.String(),
			})

		}

		if msg.Cmd == "watch" {

			offer, err := sdp.Parse(msg.Sdp)
			logger.Info("watch-offer", zap.Any("offer", offer))
			if err != nil {
				panic(err)
			}
			transport = endpoint.CreateTransport(offer, nil)
			transport.SetRemoteProperties(offer.GetMedia("audio"), offer.GetMedia("video"))

			answer := offer.Answer(
				transport.GetLocalICEInfo(),
				transport.GetLocalDTLSInfo(),
				endpoint.GetLocalCandidates(),
				Capabilities)

			transport.SetLocalProperties(answer.GetMedia("audio"), answer.GetMedia("video"))

			if incomingStream, ok := incomingStreams[msg.StreamID]; ok {
				litter.Dump(incomingStream.GetStreamInfo())
				outgoingStream := transport.CreateOutgoingStream(incomingStream.GetStreamInfo())
				outgoingStream.AttachTo(incomingStream)
				answer.AddStream(outgoingStream.GetStreamInfo())
			}

			ws.WriteJSON(Message{
				Cmd: "answer",
				Sdp: answer.String(),
			})
		}

	}
}

func publish(c *gin.Context) {
	c.HTML(http.StatusOK, "publish.html", gin.H{})
}

func watch(c *gin.Context) {
	c.HTML(http.StatusOK, "watch.html", gin.H{})
}

var logger *zap.Logger

func main() {
	logger, _ = zap.NewProduction()
	godotenv.Load()
	address := ":8000"
	if os.Getenv("port") != "" {
		address = ":" + os.Getenv("port")
	}
	r := gin.Default()
	r.LoadHTMLFiles("./publish.html", "./watch.html")
	r.GET("/channel", channel)
	r.GET("/watch/:stream", watch)
	r.GET("/", publish)
	//r.Run(address)
	r.RunTLS(address, "/home/vpsadmin/.acme.sh/tizi.sunbin123.xyz_ecc/tizi.sunbin123.xyz.cer", "/home/vpsadmin/.acme.sh/tizi.sunbin123.xyz_ecc/tizi.sunbin123.xyz.key")
}
