package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// websocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	// upgrade to ws
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	fmt.Println("player connected")

	// listen for messages
	for {
		messageType, msg, err := ws.ReadMessage()
		if err != nil {
			fmt.Println("player disconnected:", err)
			break
		}
		
		fmt.Printf("client says: %s\n", msg)
		
		// echo back
		err = ws.WriteMessage(messageType, []byte("server received your message"))
		if err != nil {
			log.Println("send error:", err)
			break
		}
	}
}

func main() {
	http.HandleFunc("/ws", handleConnections)
	fmt.Println("server running on http://localhost:8080")
	
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("server error: ", err)
	}
}