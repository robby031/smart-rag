package risk

type Scorer interface {
	ScorePayment(customerID string, amount int) int
}

type StaticScorer struct{}

func (StaticScorer) ScorePayment(customerID string, amount int) int {
	if customerID == "" || amount > 10000 {
		return 90
	}
	return 10
}
