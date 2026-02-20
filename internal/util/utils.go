package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/nogie-dev/clob-trading/internal/models"
)

func GenerateOrderID(req models.CreateOrderRequest) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%d",
		req.Ticker, req.UserID, req.OrderType, req.Position, req.Nonce)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
