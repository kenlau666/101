package engine

import (
	"errors"
	"fmt"
)

var (
	ErrUnknownEventType = errors.New("engine: unknown event type")
	ErrInvalidSide      = errors.New("engine: invalid side")
	ErrDuplicateOrderID = errors.New("engine: duplicate order id")
)

// Engine is a single-threaded deterministic matching engine.
// It must be driven from one goroutine; no time.Now() calls are made,
// no maps are iterated for matching, no floats are used.
type Engine struct {
	bids   *book
	asks   *book
	orders map[string]*Order // ID → order, used only for cancel lookup (never iterated)
	trades []Trade           // reusable output buffer
}

func New() *Engine {
	return &Engine{
		bids:   newBook(Buy),
		asks:   newBook(Sell),
		orders: make(map[string]*Order),
	}
}

// Step processes one event and returns any trades it produced.
// The returned slice is reused across calls; copy it if you need to keep it.
func (e *Engine) Step(ev Event) ([]Trade, error) {
	e.trades = e.trades[:0]
	switch ev.Type {
	case "place":
		return e.place(ev)
	case "cancel":
		return e.cancel(ev)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownEventType, ev.Type)
	}
}

func (e *Engine) place(ev Event) ([]Trade, error) {
	side, ok := parseSide(ev.Side)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidSide, ev.Side)
	}
	if _, exists := e.orders[ev.OrderID]; exists {
		return nil, fmt.Errorf("%w: %q", ErrDuplicateOrderID, ev.OrderID)
	}
	price, err := ParseDecimal(ev.Price)
	if err != nil {
		return nil, fmt.Errorf("price: %w", err)
	}
	qty, err := ParseDecimal(ev.Quantity)
	if err != nil {
		return nil, fmt.Errorf("quantity: %w", err)
	}
	taker := &Order{
		ID:        ev.OrderID,
		Side:      side,
		Price:     price,
		Quantity:  qty,
		Timestamp: ev.Timestamp,
	}

	opp := e.asks
	if side == Sell {
		opp = e.bids
	}

	for taker.Quantity > 0 {
		best := opp.best()
		if best == nil {
			break
		}
		if !crosses(side, taker.Price, best.Price) {
			break
		}
		// Walk the FIFO at this price level.
		for taker.Quantity > 0 && len(best.Orders) > 0 {
			maker := best.Orders[0]
			matchQty := maker.Quantity
			if taker.Quantity < matchQty {
				matchQty = taker.Quantity
			}
			e.trades = append(e.trades, Trade{
				TakerOrderID: taker.ID,
				MakerOrderID: maker.ID,
				Price:        FormatDecimal(best.Price),
				Quantity:     FormatDecimal(matchQty),
				Timestamp:    taker.Timestamp,
			})
			taker.Quantity -= matchQty
			maker.Quantity -= matchQty
			if maker.Quantity == 0 {
				best.Orders = best.Orders[1:]
				delete(e.orders, maker.ID)
			}
		}
		if len(best.Orders) == 0 {
			opp.removeBest()
		}
	}

	if taker.Quantity > 0 {
		var own *book
		if side == Buy {
			own = e.bids
		} else {
			own = e.asks
		}
		lvl := own.findOrInsert(taker.Price)
		lvl.Orders = append(lvl.Orders, taker)
		e.orders[taker.ID] = taker
	}
	return e.trades, nil
}

func (e *Engine) cancel(ev Event) ([]Trade, error) {
	o, ok := e.orders[ev.OrderID]
	if !ok {
		// Cancel of unknown order is a no-op — common in real exchanges
		// (the order may have already been fully filled).
		return e.trades, nil
	}
	var b *book
	if o.Side == Buy {
		b = e.bids
	} else {
		b = e.asks
	}
	b.removeOrder(o)
	delete(e.orders, o.ID)
	return e.trades, nil
}

// crosses reports whether a taker order at takerPrice on `side` would
// match against a resting maker at makerPrice.
//   buy taker crosses if maker (ask) price <= taker price
//   sell taker crosses if maker (bid) price >= taker price
func crosses(side Side, takerPrice, makerPrice int64) bool {
	if side == Buy {
		return makerPrice <= takerPrice
	}
	return makerPrice >= takerPrice
}
