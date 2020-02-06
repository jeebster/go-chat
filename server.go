package main

type Server struct {
	// Keep a list of clients
	clients map[*Client]bool

	// Client messages
	transmission chan []byte

	// Add client for message reception
	register chan *Client

	// Remove client from message reception
	unregister chan *Client
}

func newServer() *Server {
	return &Server{
		transmission: make(chan []byte),
		register: make(chan *Client),
		unregister: make(chan *Client),
		clients: make(map[*Client]bool),
	}
}

func (s *Server) start() {
	for {
		select {
		case client := <- s.register:
			s.clients[client] = true
		case client := <- s.unregister:
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				close(client.send)
			}
		case message := <- s.transmission:
			for client := range s.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(s.clients, client)
				}
			}
		}
	}
}