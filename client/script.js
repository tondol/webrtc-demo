let ws = null;
let pc = null;
let statusDiv = document.getElementById('status');
let connectBtn = document.getElementById('connectBtn');
let disconnectBtn = document.getElementById('disconnectBtn');
let remoteVideo = document.getElementById('remoteVideo');
let debugLog = document.getElementById('debugLog');

function log(message) {
    console.log(message);
    debugLog.innerHTML += new Date().toLocaleTimeString() + ': ' + message + '<br>';
    debugLog.scrollTop = debugLog.scrollHeight;
}

function updateStatus(message, type = 'connecting') {
    statusDiv.textContent = message;
    statusDiv.className = 'status ' + type;
}

function connect() {
    updateStatus('サーバーに接続中...', 'connecting');
    connectBtn.disabled = true;
    log('WebSocket接続を開始...');

    // WebSocket接続
    ws = new WebSocket('ws://localhost:8080/ws');
    
    ws.onopen = function() {
        log('WebSocket接続成功');
        updateStatus('WebRTC接続を開始中...', 'connecting');
        setupWebRTC();
    };
    
    ws.onmessage = function(event) {
        const message = JSON.parse(event.data);
        log('受信: ' + message.type);
        handleMessage(message);
    };
    
    ws.onclose = function() {
        log('WebSocket接続が切断されました');
        updateStatus('接続が切断されました', 'error');
        cleanup();
    };
    
    ws.onerror = function(error) {
        log('WebSocket接続エラー: ' + error);
        updateStatus('接続エラー', 'error');
        cleanup();
    };
}

function disconnect() {
    log('切断を開始...');
    updateStatus('切断中...', 'connecting');
    cleanup();
}

function cleanup() {
    if (pc) {
        log('WebRTC接続を閉じています...');
        pc.close();
        pc = null;
    }
    if (ws) {
        log('WebSocket接続を閉じています...');
        ws.close();
        ws = null;
    }
    remoteVideo.srcObject = null;
    updateStatus('切断されました', 'error');
    connectBtn.disabled = false;
    disconnectBtn.disabled = true;
    log('クリーンアップ完了');
}

function setupWebRTC() {
    log('WebRTC Peer Connectionを作成中...');
    
    // WebRTC設定
    const config = {
        iceServers: [
            { urls: 'stun:stun.l.google.com:19302' }
        ]
    };

    pc = new RTCPeerConnection(config);
    log('Peer Connection作成完了');

    // リモートストリーム受信
    pc.ontrack = function(event) {
        log('リモートトラックを受信 - 種類: ' + event.track.kind);
        
        if (event.streams && event.streams[0]) {
            log('ビデオストリームを設定中...');
            remoteVideo.srcObject = event.streams[0];
            
            // ビデオ再生開始時
            remoteVideo.addEventListener('playing', function() {
                log('ビデオ再生開始');
                updateStatus('カメラ映像を再生中', 'connected');
            }, { once: true });
            
            // ビデオエラー時
            remoteVideo.addEventListener('error', function(e) {
                log('ビデオエラー: ' + e.error);
            }, { once: true });
            
            updateStatus('カメラ映像を受信中', 'connected');
            disconnectBtn.disabled = false;
        } else {
            log('ストリーム受信に失敗しました');
        }
    };

    // ICE候補の処理とOfferの送信
    pc.onicecandidate = function(event) {
        if (event.candidate) {
            log('ICE候補を送信中...');
            sendMessage({
                type: 'ice-candidate',
                data: JSON.stringify(event.candidate)
            });
        } else {
            log('ICE gathering完了 - Offerを送信中...');
            // ICE gathering完了 - Offerを送信
            if (pc.localDescription) {
                sendMessage({
                    type: 'offer',
                    data: JSON.stringify(pc.localDescription)
                });
            }
        }
    };

    // 接続状態の監視
    pc.onconnectionstatechange = function() {
        log('接続状態変更: ' + pc.connectionState);
        if (pc.connectionState === 'connected') {
            updateStatus('WebRTC接続完了', 'connected');
        } else if (pc.connectionState === 'failed') {
            updateStatus('接続に失敗しました', 'error');
            log('WebRTC接続失敗: ' + pc.connectionState);
        }
    };

    // ICE接続状態の監視
    pc.oniceconnectionstatechange = function() {
        log('ICE接続状態: ' + pc.iceConnectionState);
    };

    // オファーを作成
    const offerOptions = {
        offerToReceiveVideo: true,
        offerToReceiveAudio: false
    };
    
    log('Offerを作成中...');
    pc.createOffer(offerOptions)
        .then(function(offer) {
            log('Offer作成成功 - ローカル記述を設定中...');
            return pc.setLocalDescription(offer);
        })
        .then(function() {
            log('ローカル記述設定完了');
        })
        .catch(function(error) {
            console.error('Error creating offer:', error);
            updateStatus('オファー作成エラー', 'error');
            log('オファー作成エラー: ' + error);
        });
}

function handleMessage(message) {
    switch (message.type) {
        case 'answer':
            log('Answerを受信 - リモート記述を設定中...');
            const answer = JSON.parse(message.data);
            pc.setRemoteDescription(new RTCSessionDescription(answer))
                .then(function() {
                    log('リモート記述設定完了');
                })
                .catch(function(error) {
                    console.error('Error setting remote description:', error);
                    updateStatus('接続設定エラー', 'error');
                    log('リモート記述設定エラー: ' + error);
                });
            break;
            
        case 'ice-candidate':
            log('ICE候補を受信中...');
            const candidate = JSON.parse(message.data);
            pc.addIceCandidate(new RTCIceCandidate(candidate))
                .then(function() {
                    log('ICE候補追加成功');
                })
                .catch(function(error) {
                    log('ICE候補追加エラー: ' + error);
                });
            break;
    }
}

function sendMessage(message) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(message));
        log('送信: ' + message.type);
    } else {
        log('WebSocket未接続 - メッセージ送信失敗: ' + message.type);
    }
}

// DOM読み込み完了時に初期化
document.addEventListener('DOMContentLoaded', function() {
    log('DOM読み込み完了 - 初期化中...');
    
    statusDiv = document.getElementById('status');
    connectBtn = document.getElementById('connectBtn');
    disconnectBtn = document.getElementById('disconnectBtn');
    remoteVideo = document.getElementById('remoteVideo');
    debugLog = document.getElementById('debugLog');
    
    // イベントリスナーを追加
    connectBtn.addEventListener('click', connect);
    disconnectBtn.addEventListener('click', disconnect);
    
    log('アプリケーション初期化完了');
});

// ページを離れる時のクリーンアップ
window.addEventListener('beforeunload', function() {
    cleanup();
});
