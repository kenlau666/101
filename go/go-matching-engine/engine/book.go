package engine

import "sort"

// priceLevel holds all resting orders at one price, in arrival order (FIFO).
type priceLevel struct {
	Price  int64
	Orders []*Order
}

// book is one side of the order book: a price-ordered slice of priceLevels.
// For bids, ordered descending by Price (best bid = highest price = index 0).
// For asks, ordered ascending by Price (best ask = lowest price = index 0).
//
// Sorted slices satisfy the "no maps" determinism requirement: iteration
// order is fully determined by price (and arrival order within a level),
// not by Go map randomization.
type book struct {
	levels []*priceLevel
	side   Side
}

func newBook(s Side) *book { return &book{side: s} }

// betterPrice reports whether price a is strictly better than b on this side.
// For bids: higher is better. For asks: lower is better.
func (b *book) betterPrice(a, c int64) bool {
	if b.side == Buy {
		return a > c
	}
	return a < c
}

// findOrInsert returns the priceLevel for `price`, creating it if needed
// while keeping `levels` sorted.
func (b *book) findOrInsert(price int64) *priceLevel {
	i := sort.Search(len(b.levels), func(i int) bool {
		// First index where level.Price is NOT strictly better than `price`.
		return !b.betterPrice(b.levels[i].Price, price)
	})
	if i < len(b.levels) && b.levels[i].Price == price {
		return b.levels[i]
	}
	lvl := &priceLevel{Price: price}
	b.levels = append(b.levels, nil)
	copy(b.levels[i+1:], b.levels[i:])
	b.levels[i] = lvl
	return lvl
}

// best returns the best (front) priceLevel, or nil if the book is empty.
func (b *book) best() *priceLevel {
	if len(b.levels) == 0 {
		return nil
	}
	return b.levels[0]
}

// removeBest removes the front level; called when its FIFO is empty.
func (b *book) removeBest() {
	b.levels = b.levels[1:]
}

// removeOrder removes a specific order from its price level. Returns
// false if the order isn't there. O(N) within the level, O(log L) to find.
func (b *book) removeOrder(o *Order) bool {
	i := sort.Search(len(b.levels), func(i int) bool {
		return !b.betterPrice(b.levels[i].Price, o.Price)
	})
	if i >= len(b.levels) || b.levels[i].Price != o.Price {
		return false
	}
	lvl := b.levels[i]
	for j, ord := range lvl.Orders {
		if ord == o {
			lvl.Orders = append(lvl.Orders[:j], lvl.Orders[j+1:]...)
			if len(lvl.Orders) == 0 {
				b.levels = append(b.levels[:i], b.levels[i+1:]...)
			}
			return true
		}
	}
	return false
}
