package domain

// Department is the business unit that owns an agent or requests one.
type Department string

const (
	DepartmentPrepress           Department = "PREPRESS"
	DepartmentCustomerExperience Department = "CUSTOMER_EXPERIENCE"
	DepartmentOperations         Department = "OPERATIONS"
	DepartmentMarketing          Department = "MARKETING"
	DepartmentFinance            Department = "FINANCE"
)

// Departments lists every department in display order.
var Departments = []Department{
	DepartmentPrepress, DepartmentCustomerExperience, DepartmentOperations,
	DepartmentMarketing, DepartmentFinance,
}

func (d Department) Valid() bool {
	for _, known := range Departments {
		if d == known {
			return true
		}
	}
	return false
}
