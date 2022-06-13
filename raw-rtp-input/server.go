package main

import "C"

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/notedit/gst"
	mediaserver "github.com/notedit/media-server-go"
	"github.com/notedit/sdp"
)

const (
	videoPt    = 100
	audioPt    = 96
	videoCodec = "h264"
	audioCodec = "opus"
)

type Message struct {
	Cmd string `json:"cmd,omitempty"`
	Sdp string `json:"sdp,omitempty"`
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
		Codecs: []string{"h264"},
		Rtx:    true,
		Rtcpfbs: []*sdp.RtcpFeedback{
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
		},
	},
}

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

		if msg.Cmd == "offer" {
			offer, err := sdp.Parse(msg.Sdp)
			if err != nil {
				panic(err)
			}
			transport = endpoint.CreateTransport(offer, nil)
			transport.SetRemoteProperties(offer.GetMedia("audio"), offer.GetMedia("video"))

			answer := offer.Answer(transport.GetLocalICEInfo(),
				transport.GetLocalDTLSInfo(),
				endpoint.GetLocalCandidates(),
				Capabilities)

			videoSession := mediaserver.NewMediaFrameSession(offer.GetMedia("video"))
			audioSession := mediaserver.NewMediaFrameSession(offer.GetMedia("audio"))

			videoCodecInfo := offer.GetMedia("video").GetCodec(videoCodec)
			// audioCodecInfo := offer.GetMedia("audio").GetCodec(audioCodec)

			transport.SetLocalProperties(answer.GetMedia("audio"), answer.GetMedia("video"))

			outgoingStream := transport.CreateOutgoingStreamWithID(uuid.Must(uuid.NewV4()).String(), true, true)

			outgoingStream.GetVideoTracks()[0].AttachTo(videoSession.GetIncomingStreamTrack())
			outgoingStream.GetAudioTracks()[0].AttachTo(audioSession.GetIncomingStreamTrack())

			go generteVideoRTP(videoSession, videoCodecInfo.GetType())
			//go generateAudioRTP(audioSession, audioCodecInfo.GetType())

			info := outgoingStream.GetStreamInfo()
			answer.AddStream(info)

			ws.WriteJSON(Message{
				Cmd: "answer",
				Sdp: answer.String(),
			})
		}
	}
}

func generteVideoRTP(session *mediaserver.MediaFrameSession, payload int) {

	pipelineStr := "videotestsrc is-live=true ! video/x-raw,format=I420,framerate=15/1 ! x264enc aud=false bframes=0 speed-preset=veryfast key-int-max=15 ! video/x-h264,stream-format=byte-stream,profile=baseline ! h264parse ! rtph264pay config-interval=-1  pt=%d ! appsink name=appsink"
	pipelineStr = fmt.Sprintf(pipelineStr, payload)
	pipeline, err := gst.ParseLaunch(pipelineStr)

	if err != nil {
		panic("can not create pipeline")
	}

	fmt.Println(pipelineStr)

	appsink := pipeline.GetByName("appsink")
	pipeline.SetState(gst.StatePlaying)

	for {
		sample, err := appsink.PullSample()
		if err != nil {
			if appsink.IsEOS() == true {
				fmt.Println("eos")
				return
			} else {
				fmt.Println(err)
				continue
			}
		}

		session.Push(sample.Data)
	}
}

func generateAudioRTP(session *mediaserver.MediaFrameSession, payload int) {

	pipelineStr := "filesrc location=output.aac ! decodebin ! audioconvert ! audioresample ! opusenc ! rtpopuspay pt=%d ! appsink name=appsink"
	pipelineStr = fmt.Sprintf(pipelineStr, payload)

	pipeline, err := gst.ParseLaunch(pipelineStr)

	if err != nil {
		panic("can not create pipeline")
	}

	fmt.Println(pipelineStr)

	appsink := pipeline.GetByName("appsink")
	pipeline.SetState(gst.StatePlaying)

	for {
		sample, err := appsink.PullSample()
		if err != nil {
			if appsink.IsEOS() == true {
				fmt.Println("eos")
				return
			} else {
				fmt.Println(err)
				continue
			}
		}

		session.Push(sample.Data)
	}
}

func index(c *gin.Context) {
	fmt.Println("helloworld")
	c.HTML(http.StatusOK, "index.html", gin.H{})
}

func main() {

	err := gst.CheckPlugins([]string{"x264", "rtp", "videoparsersbad"})

	if err != nil {
		fmt.Println(err)
		return
	}

	godotenv.Load()
	mediaserver.EnableDebug(true)
	mediaserver.EnableLog(true)
	mediaserver.EnableUltraDebug(true)

	address := ":8000"
	if os.Getenv("port") != "" {
		address = ":" + os.Getenv("port")
	}
	r := gin.Default()
	r.LoadHTMLFiles("./index.html")
	r.GET("/channel", channel)
	r.GET("/", index)
	//r.Run(address)
	r.RunTLS(address, "/home/vpsadmin/.acme.sh/tizi.sunbin123.xyz_ecc/tizi.sunbin123.xyz.cer", "/home/vpsadmin/.acme.sh/tizi.sunbin123.xyz_ecc/tizi.sunbin123.xyz.key")
}
