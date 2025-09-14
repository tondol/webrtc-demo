package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

type ConnectionInfo struct {
	pc        *webrtc.PeerConnection
	ctx       context.Context
	cancel    context.CancelFunc
	ffmpegCmd *exec.Cmd
	wsMutex   sync.Mutex // WebSocket書き込み用
}

type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // 本番環境では適切なチェックを行う
		},
	}
	connections = make(map[*websocket.Conn]*ConnectionInfo)
	connMutex   sync.RWMutex
)

// Helper function for sending WebSocket messages
func sendMessage(conn *websocket.Conn, msgType string, data interface{}) {
	connMutex.RLock()
	connInfo, exists := connections[conn]
	connMutex.RUnlock()

	if !exists {
		return
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal %s: %v", msgType, err)
		return
	}

	connInfo.wsMutex.Lock()
	defer connInfo.wsMutex.Unlock()

	conn.WriteJSON(Message{
		Type: msgType,
		Data: string(dataJSON),
	})
}

func main() {
	// HTTP server setup
	http.HandleFunc("/ws", handleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("./client/")))

	fmt.Println("WebRTC server starting on :8080")
	fmt.Println("Open http://localhost:8080 in your browser")

	log.Fatal(http.ListenAndServe(":8080", nil))
}

// ICEサーバー設定（決め打ち）
func createICEServers() []webrtc.ICEServer {
	return []webrtc.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
		{URLs: []string{"stun:stun1.l.google.com:19302"}},
		// 必要に応じてTURNサーバーを追加
		// {
		// 	URLs:       []string{"turn:your-turn-server.com:3478"},
		// 	Username:   "your-username",
		// 	Credential: "your-password",
		// },
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	fmt.Printf("New WebSocket connection from %s\n", conn.RemoteAddr())

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			break
		}

		switch msg.Type {
		case "offer":
			handleOffer(conn, msg.Data)
		case "ice-candidate":
			handleICECandidate(conn, msg.Data)
		}
	}

	cleanupConnection(conn)
}

// Connection cleanup logic
func cleanupConnection(conn *websocket.Conn) {
	connMutex.Lock()
	defer connMutex.Unlock()

	connInfo, exists := connections[conn]
	if !exists {
		return
	}

	// FFmpegプロセス終了
	if connInfo.ffmpegCmd != nil && connInfo.ffmpegCmd.Process != nil {
		connInfo.ffmpegCmd.Process.Signal(os.Interrupt)
		time.Sleep(100 * time.Millisecond)
		connInfo.ffmpegCmd.Process.Kill()
	}

	// コンテキストキャンセル・接続クリーンアップ
	connInfo.cancel()
	connInfo.pc.Close()
	delete(connections, conn)
}

func handleOffer(conn *websocket.Conn, data interface{}) {
	offerStr, ok := data.(string)
	if !ok {
		log.Println("Invalid offer format")
		return
	}

	// WebRTC peer connection configuration with ICE servers
	config := webrtc.Configuration{
		ICEServers: createICEServers(),
	}

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Printf("Failed to create peer connection: %v", err)
		return
	}

	// Store connection with context
	ctx, cancel := context.WithCancel(context.Background())
	connInfo := &ConnectionInfo{
		pc:     peerConnection,
		ctx:    ctx,
		cancel: cancel,
	}

	connMutex.Lock()
	connections[conn] = connInfo
	connMutex.Unlock()

	// Create and add video track
	videoTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
	if err != nil {
		log.Printf("Failed to create video track: %v", err)
		return
	}

	rtpSender, err := peerConnection.AddTrack(videoTrack)
	if err != nil {
		log.Printf("Failed to add track: %v", err)
		return
	}

	// Handle RTCP packets
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	// Handle ICE candidates
	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		sendMessage(conn, "ice-candidate", c.ToJSON())
	})

	// Parse and set remote description
	var offer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(offerStr), &offer); err != nil {
		log.Printf("Failed to unmarshal offer: %v", err)
		return
	}

	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		log.Printf("Failed to set remote description: %v", err)
		return
	}

	// Create and send answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		log.Printf("Failed to create answer: %v", err)
		return
	}

	if err := peerConnection.SetLocalDescription(answer); err != nil {
		log.Printf("Failed to set local description: %v", err)
		return
	}

	sendMessage(conn, "answer", answer)

	// Start streaming
	// go streamFFmpegTestPattern(videoTrack, connInfo)
	go streamCameraVP8(videoTrack, connInfo)
}

func handleICECandidate(conn *websocket.Conn, data interface{}) {
	candidateStr, ok := data.(string)
	if !ok {
		return
	}

	connMutex.RLock()
	connInfo, exists := connections[conn]
	connMutex.RUnlock()

	if !exists {
		return
	}

	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(candidateStr), &candidate); err != nil {
		return
	}

	connInfo.pc.AddICECandidate(candidate)
}

func streamFFmpegTestPattern(track *webrtc.TrackLocalStaticSample, connInfo *ConnectionInfo) {
	// FFmpegコマンド設定
	args := []string{
		"-f", "lavfi",
		"-i", "testsrc=size=640x480:rate=10",
		"-vcodec", "libvpx",
		"-deadline", "realtime",
		"-f", "ivf",
		"-",
	}

	streamFFmpeg(track, connInfo, "test pattern", args, time.Second/10)
}

func streamCameraVP8(track *webrtc.TrackLocalStaticSample, connInfo *ConnectionInfo) {
	// FFmpegコマンド設定（高画質版）
	args := []string{
		// 入力設定
		"-f", "avfoundation",
		"-framerate", "30",
		"-video_size", "1280x720", // カメラ側での解像度指定
		"-pixel_format", "uyvy422", // 高品質ピクセルフォーマット
		"-i", "0",

		// エンコード設定
		"-vcodec", "libvpx",
		"-b:v", "1000k",
		"-deadline", "realtime",
		"-f", "ivf",
		"-",
	}

	streamFFmpeg(track, connInfo, "camera", args, time.Second/30)
}

// FFmpeg streaming logic (共通処理)
func streamFFmpeg(track *webrtc.TrackLocalStaticSample, connInfo *ConnectionInfo, streamType string, args []string, frameDuration time.Duration) {
	fmt.Printf("Starting %s streaming (VP8)...\n", streamType)

	cmd := exec.CommandContext(connInfo.ctx, "ffmpeg", args...)
	connInfo.ffmpegCmd = cmd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to create stdout pipe: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start FFmpeg: %v", err)
		return
	}

	// プロセス終了監視
	go func() {
		<-connInfo.ctx.Done()
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt)
			time.Sleep(100 * time.Millisecond)
			cmd.Process.Kill()
		}
	}()

	// IVFヘッダー読み飛ばし
	header := make([]byte, 32)
	if _, err := stdout.Read(header); err != nil {
		log.Printf("Failed to read IVF header: %v", err)
		return
	}

	// フレーム処理ループ
	frameHeader := make([]byte, 12)
	for {
		select {
		case <-connInfo.ctx.Done():
			return
		default:
		}

		// フレームヘッダー読み取り
		if n, err := stdout.Read(frameHeader); err != nil || n < 12 {
			break
		}

		// フレームサイズ取得
		frameSize := uint32(frameHeader[0]) | uint32(frameHeader[1])<<8 |
			uint32(frameHeader[2])<<16 | uint32(frameHeader[3])<<24

		if frameSize > 10_000_000 { // 10MB以下を期待
			continue
		}

		// フレームデータ読み取り
		// 1回で65536バイト以上読めないため複数回readする
		frameData := make([]byte, frameSize)
		n := 0
		for n < int(frameSize) {
			m, err := stdout.Read(frameData[n:])
			if err != nil {
				break
			}
			n += m
		}

		// フレーム送信
		if err := track.WriteSample(media.Sample{
			Data:     frameData,
			Duration: frameDuration,
		}); err != nil {
			break
		}
	}
}
