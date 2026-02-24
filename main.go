package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// 用户结构
type User struct {
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	WS       *websocket.Conn
}

// 消息结构（对齐 Telegram 消息字段）
type Message struct {
	ID        int64     `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	IsRead    bool      `json:"is_read"`
	Avatar    string    `json:"avatar"`
}

// 会话结构
type Session struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	IsGroup  bool   `json:"is_group"`
	LastMsg  string `json:"last_msg"`
	LastTime time.Time `json:"last_time"`
	Unread   int    `json:"unread"`
}

var (
	users     = make(map[string]*User)
	messages  []Message
	sessions  []Session
	userMu    sync.Mutex
	msgMu     sync.Mutex
	msgID     int64 = 1
)

// 初始化默认公共聊天室
func init() {
	sessions = append(sessions, Session{
		ID:       "public-chat",
		Name:     "公共聊天室",
		Avatar:   "https://img.icons8.com/fluency/96/000000/chat.png",
		IsGroup:  true,
		LastMsg:  "欢迎加入公共聊天室",
		LastTime: time.Now(),
	})
}

// 广播消息给所有在线用户
func broadcast(msg Message) {
	userMu.Lock()
	defer userMu.Unlock()
	for _, u := range users {
		if u.Username == msg.From {
			continue
		}
		_ = websocket.JSON.Send(u.WS, msg)
	}
}

// WebSocket 处理连接
func wsHandler(ws *websocket.Conn) {
	defer ws.Close()

	// 握手获取用户名
	var username string
	if err := websocket.Message.Receive(ws, &username); err != nil || username == "" {
		return
	}

	// 注册用户
	userMu.Lock()
	users[username] = &User{
		Username: username,
		Avatar:   string(username[0]),
		WS:       ws,
	}
	userMu.Unlock()

	// 退出时注销用户
	defer func() {
		userMu.Lock()
		delete(users, username)
		userMu.Unlock()
	}()

	// 循环接收消息
	for {
		var msg Message
		if err := websocket.JSON.Receive(ws, &msg); err != nil {
			break
		}

		// 填充消息信息
		msgMu.Lock()
		msg.ID = msgID
		msgID++
		msg.Timestamp = time.Now()
		msg.IsRead = false
		msg.Avatar = string(msg.From[0])
		messages = append(messages, msg)
		msgMu.Unlock()

		// 更新会话最后一条消息
		for i, s := range sessions {
			if s.ID == msg.To {
				sessions[i].LastMsg = msg.Content
				sessions[i].LastTime = msg.Timestamp
				break
			}
		}

		// 广播消息
		broadcast(msg)
		// 回发给发送者
		_ = websocket.JSON.Send(ws, msg)
	}
}

// 获取会话列表
func sessionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sessions)
}

// 获取历史消息
func messagesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	msgMu.Lock()
	var res []Message
	for _, msg := range messages {
		if msg.To == sessionID {
			res = append(res, msg)
		}
	}
	msgMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// 首页
func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func main() {
	// 路由
	http.HandleFunc("/", indexHandler)
	http.Handle("/ws", websocket.Handler(wsHandler))
	http.HandleFunc("/api/sessions", sessionsHandler)
	http.HandleFunc("/api/messages", messagesHandler)

	// 端口适配
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("服务启动在 http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
