package payments

import "example.com/search-eval/internal/risk"

type PaymentProcessor struct {
	riskScorer risk.Scorer
}

func NewPaymentProcessor(riskScorer risk.Scorer) *PaymentProcessor {
	return &PaymentProcessor{riskScorer: riskScorer}
}

func (p *PaymentProcessor) ChargeCustomer(customerID string, amount int) error {
	score := p.riskScorer.ScorePayment(customerID, amount)
	if score > 80 {
		return ErrHighRiskPayment
	}
	return nil
}

func ProcessRefund(paymentID string, amount int) error {
	if paymentID == "" || amount <= 0 {
		return ErrInvalidRefund
	}
	return nil
}
