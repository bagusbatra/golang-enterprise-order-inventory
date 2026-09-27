package order

// Status merepresentasikan seluruh nilai valid orders.status (spec Section
// 16, database-contract.md Section 8).
type Status string

const (
	StatusPending        Status = "PENDING"
	StatusWaitingPayment Status = "WAITING_PAYMENT"
	StatusPaid           Status = "PAID"
	StatusProcessing     Status = "PROCESSING"
	StatusPacked         Status = "PACKED"
	StatusShipped        Status = "SHIPPED"
	StatusCompleted      Status = "COMPLETED"
	StatusCancelled      Status = "CANCELLED"
	StatusExpired        Status = "EXPIRED"
)

// transitions adalah whitelist EKSPLISIT (spec Section 16) — satu-satunya
// sumber kebenaran urutan status order di seluruh sistem (order/payment/
// shipment). Apa pun yang tidak terdaftar di sini otomatis ilegal, termasuk
// keempat kasus yang spec sebut eksplisit "tidak boleh": COMPLETED->PENDING,
// SHIPPED->PENDING, CANCELLED->PAID, EXPIRED->SHIPPED.
var transitions = map[Status]map[Status]bool{
	StatusPending: {
		StatusWaitingPayment: true,
		StatusCancelled:      true,
	},
	StatusWaitingPayment: {
		StatusPaid:      true,
		StatusExpired:   true,
		StatusCancelled: true,
	},
	StatusPaid: {
		StatusProcessing: true,
		StatusCancelled:  true,
	},
	StatusProcessing: {
		StatusPacked: true,
	},
	StatusPacked: {
		StatusShipped: true,
	},
	StatusShipped: {
		StatusCompleted: true,
	},
}

// CanTransition adalah SATU-SATUNYA tempat yang boleh memutuskan apakah
// perpindahan status order valid. WAJIB dipanggil di SETIAP perubahan status
// order di seluruh modul Agent 3 (order/payment/shipment) — jangan pernah
// menulis `order.Status = "X"` langsung tanpa lewat fungsi ini (spec Section
// 16: "Semua transition harus divalidasi service").
func CanTransition(from, to Status) bool {
	allowed, ok := transitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}
