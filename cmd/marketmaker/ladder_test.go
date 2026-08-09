package main

import "testing"

// Whenever a sub-step crosses the ladder, the bot holding the bids pays quote
// currency and the bot holding the asks receives it — in both price directions,
// since a fall hits the bids and a rise lifts the asks. So a fixed side
// assignment drains one bot's quote and the other's base asset monotonically,
// which emptied both sides of the book after about a week of running.
//
// These assert the property that makes the flow reverse instead.
func TestLadderSidesAlternateBetweenTicks(t *testing.T) {
	for tickNum := range 8 {
		bidder, asker := ladderSides(tickNum)
		if bidder == asker {
			t.Fatalf("tick %d put %s on both sides", tickNum, bidder)
		}

		nextBidder, _ := ladderSides(tickNum + 1)
		if bidder == nextBidder {
			t.Errorf("ticks %d and %d both put %s on the bid side; the drain does not reverse",
				tickNum, tickNum+1, bidder)
		}
	}
}

// Over an even number of ticks each bot must take each side equally often, or
// the flow still has a net direction, just a slower one.
func TestLadderSidesAreBalancedOverManyTicks(t *testing.T) {
	bids := map[string]int{}
	asks := map[string]int{}
	for tickNum := range 1000 {
		bidder, asker := ladderSides(tickNum)
		bids[bidder]++
		asks[asker]++
	}

	if bids[bots[0]] != bids[bots[1]] {
		t.Errorf("bid side split unevenly over 1000 ticks: %v", bids)
	}
	if asks[bots[0]] != asks[bots[1]] {
		t.Errorf("ask side split unevenly over 1000 ticks: %v", asks)
	}
}
