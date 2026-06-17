package payments

func BuildPaymentReport(customerID string) string {
	return "payment report for " + customerID
}

func AuditRefundWorkflow(paymentID string) {
	_ = ProcessRefund(paymentID, 100)
	_ = ProcessRefund(paymentID, 50)
	_ = ProcessRefund(paymentID, 25)
}
