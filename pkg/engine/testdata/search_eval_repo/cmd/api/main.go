package main

import (
	"example.com/search-eval/internal/auth"
	"example.com/search-eval/internal/payments"
	"example.com/search-eval/internal/risk"
)

func main() {
	processor := payments.NewPaymentProcessor(risk.StaticScorer{})
	_ = processor.ChargeCustomer("customer-1", 1200)
	_ = payments.ProcessRefund("payment-1", 100)
	_ = auth.ValidateSession("session-token")
}
