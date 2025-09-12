# WebRTC Camera Stream Demo

Go言語（Pion WebRTC）とHTML/JavaScriptを使用したWebRTCカメラストリーミングのミニマルデモです。

## 機能

- **サーバー**: Go言語で実装、FFmpegテストパターンまたはカメラをVP8でWebRTC配信
- **クライアント**: ブラウザでサーバーに接続し、映像を受信・再生
- **プロセス管理**: 接続切断時のFFmpegプロセス適切終了
- **TURNサーバー対応**: NAT/ファイアウォール環境での接続をサポート

## 前提条件

- Go 1.21以上
- FFmpeg（映像キャプチャとVP8エンコーディング用）
- macOS（現在の実装はmacOSのカメラ用、テストパターンは全OS対応）

### FFmpegのインストール

```bash
# Homebrewを使用してインストール
brew install ffmpeg
```

## 使用方法

### 1. サーバーの起動

```bash
# プロジェクトディレクトリに移動
cd /path/to/webrtc-demo

# 依存関係のダウンロード
go mod tidy

# サーバーの起動
go run server/main.go
```

サーバーが起動すると、以下のようなメッセージが表示されます：
```
WebRTC server starting on :8080
Open http://localhost:8080 in your browser
```

### 2. クライアントでの接続

1. ブラウザで `http://localhost:8080` を開く
2. 「接続」ボタンをクリック
3. カメラ映像が表示されることを確認

## プロジェクト構造

```
webrtc-demo/
├── go.mod              # Go モジュール設定
├── server/
│   └── main.go         # WebRTCサーバー実装
├── client/
│   ├── index.html      # HTMLファイル
│   ├── style.css       # CSSスタイルシート
│   └── script.js       # JavaScript実装
└── README.md           # このファイル
```

## 映像配信の選択

デフォルトではテストパターンが配信されます。実際のカメラを使用したい場合：

1. `server/main.go`の`handleOffer`関数で以下を変更：
```go
go streamFFmpegTestPattern(videoTrack, connInfo)
// go streamCameraVP8(videoTrack, connInfo)
```

2. カメラ配信を有効にする場合はコメントを切り替え：
```go
// go streamFFmpegTestPattern(videoTrack, connInfo)
go streamCameraVP8(videoTrack, connInfo)
```

## 技術仕様

### サーバー側
- **言語**: Go
- **WebRTCライブラリ**: Pion WebRTC v3.3.6
- **WebSocketライブラリ**: Gorilla WebSocket v1.5.3
- **動画エンコーディング**: FFmpeg (VP8)
- **映像形式**: IVF（VP8コンテナ）
- **プロセス管理**: Context-based cancellation

#### テストパターン設定
- **解像度**: 640x480
- **フレームレート**: 10 FPS

#### カメラ設定（高画質版）
- **解像度**: 1280x720 (720p)
- **フレームレート**: 30 FPS

### クライアント側
- **技術**: HTML5, CSS3, JavaScript (ES6+), WebRTC API
- **ブラウザサポート**: モダンブラウザ（Chrome, Firefox, Safari, Edge）
- **ファイル分離**: HTML/CSS/JavaScript完全分離

## ICEサーバー設定

### デフォルト設定
```go
// server/main.go の createICEServers() 関数
[]webrtc.ICEServer{
    {URLs: []string{"stun:stun.l.google.com:19302"}},
    {URLs: []string{"stun:stun1.l.google.com:19302"}},
}
```

### TURNサーバー追加
NAT/ファイアウォール環境で接続が困難な場合、TURNサーバーを追加：

```go
// createICEServers() 関数のコメントアウト部分を編集
{
    URLs:       []string{"turn:your-turn-server.com:3478"},
    Username:   "your-username", 
    Credential: "your-password",
},
```

## シグナリングプロトコル

WebSocketを使用したJSONメッセージ：

```json
// Offer (Client → Server)
{
  "type": "offer",
  "data": "{\"type\":\"offer\",\"sdp\":\"...\"}"
}

// Answer (Server → Client)
{
  "type": "answer", 
  "data": "{\"type\":\"answer\",\"sdp\":\"...\"}"
}

// ICE Candidate (Bidirectional)
{
  "type": "ice-candidate",
  "data": "{\"candidate\":\"...\",\"sdpMLineIndex\":0,\"sdpMid\":\"...\"}"
}
```

## 開発とデバッグ

### FFmpeg単体テスト

#### カメラデバイス確認
```bash
ffmpeg -f avfoundation -list_devices true -i ""
```

#### テストパターン生成テスト
```bash
ffmpeg -f lavfi -i "testsrc=size=640x480:rate=10" -vcodec libvpx -cpu-used 16 -deadline 1 -g 10 -f ivf -t 5 test.ivf
```

#### カメラキャプチャテスト
```bash
ffmpeg -f avfoundation -framerate 30 -video_size 1280x720 -i 0 -vcodec libvpx -cpu-used 8 -deadline realtime -t 5 camera_test.ivf
```

### サーバーログ
- WebSocket接続状況
- WebRTC接続状態
- FFmpegプロセス管理状況

### クライアントデバッグ
- ブラウザ開発者ツールのコンソールログ
- ページ内デバッグログエリア
- WebRTC接続状態の詳細表示

## トラブルシューティング

### カメラにアクセスできない場合
- macOSの場合、ターミナルアプリにカメラアクセス許可を与える必要があります
- システム設定 > プライバシーとセキュリティ > カメラ で確認
- カメラが他のアプリで使用中でないか確認

### FFmpegエラーが発生する場合
- FFmpegがインストールされているか確認: `ffmpeg -version`
- カメラデバイスが利用可能か確認: `ffmpeg -f avfoundation -list_devices true -i ""`
- VP8エンコーダーが利用可能か確認: `ffmpeg -encoders | grep libvpx`

### WebRTC接続が失敗する場合
- ブラウザでHTTPS（localhost除く）を使用していることを確認
- ファイアウォール設定を確認
- ブラウザの開発者ツールでエラーメッセージを確認
- ICE gathering の状態をログで確認
- TURNサーバーの設定を検討

## ライセンス

このプロジェクトはMITライセンスの下で公開されています。
