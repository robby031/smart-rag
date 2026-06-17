package payments

import "errors"

var ErrHighRiskPayment = errors.New("high risk payment")

var ErrInvalidRefund = errors.New("invalid refund")
