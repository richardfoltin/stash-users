package session

type SessionConfig interface {
	GetUsername() string
	GetAPIKey() string

	GetSessionStoreKey() []byte
	GetMaxSessionAge() int
}
