package engine

type Side uint8

const (
	Buy Side = iota
	Sell
)

func parseSide(s string) (Side, bool) {
	switch s {
	case "buy":
		return Buy, true
	case "sell":
		return Sell, true
	}
	return 0, false
}

// Event is the wire format for input events. Price/Quantity are decimal
// strings; the engine converts them to fixed-point int64 internally.
type Event struct {
	Type      string `json:"type"`
	OrderID   string `json:"order_id"`
	Side      string `json:"side,omitempty"`
	Price     string `json:"price,omitempty"`
	Quantity  string `json:"quantity,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// Order is the engine's internal representation. Price and Quantity are
// fixed-point int64 at scale 10^8.
type Order struct {
	ID        string
	Side      Side
	Price     int64
	Quantity  int64 // remaining quantity
	Timestamp int64
}

// Trade is the wire format for output trades. Price/Quantity are decimal
// strings produced by FormatDecimal.
type Trade struct {
	TakerOrderID string `json:"taker_order_id"`
	MakerOrderID string `json:"maker_order_id"`
	Price        string `json:"price"`
	Quantity     string `json:"quantity"`
	Timestamp    int64  `json:"timestamp"`
}
