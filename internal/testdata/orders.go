package testdata

import "github.com/nogie-dev/clob-trading/internal/models"

var SampleOrders = []models.CreateOrderRequest{
	// Bids
	{
		Ticker:    "BTC-USD",
		UserID:    "nogie",
		OrderType: models.Limit,
		Position:  models.Bid,
		Price:     96536.2,
		Amount:    0.05,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "alice",
		OrderType: models.Limit,
		Position:  models.Bid,
		Price:     96535.8,
		Amount:    0.03,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "bob",
		OrderType: models.Limit,
		Position:  models.Bid,
		Price:     96535.0,
		Amount:    0.07,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "carol",
		OrderType: models.Limit,
		Position:  models.Bid,
		Price:     96534.5,
		Amount:    0.02,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "dave",
		OrderType: models.Limit,
		Position:  models.Bid,
		Price:     96533.9,
		Amount:    0.06,
	},

	// Asks
	{
		Ticker:    "BTC-USD",
		UserID:    "eve",
		OrderType: models.Limit,
		Position:  models.Ask,
		Price:     96536.2,
		Amount:    0.05,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "frank",
		OrderType: models.Limit,
		Position:  models.Ask,
		Price:     96537.0,
		Amount:    0.04,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "grace",
		OrderType: models.Limit,
		Position:  models.Ask,
		Price:     96537.8,
		Amount:    0.03,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "heidi",
		OrderType: models.Limit,
		Position:  models.Ask,
		Price:     96538.5,
		Amount:    0.02,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "heidi",
		OrderType: models.Limit,
		Position:  models.Ask,
		Price:     96538.5,
		Amount:    1.02,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "heidi",
		OrderType: models.Limit,
		Position:  models.Ask,
		Price:     96538.5,
		Amount:    4.02,
	},
	{
		Ticker:    "BTC-USD",
		UserID:    "ivan",
		OrderType: models.Limit,
		Position:  models.Ask,
		Price:     96539.3,
		Amount:    0.06,
	},
}
