package tickets

// GetTotalTicketsCount returns the total count of tickets
func (s *TicketService) GetTotalTicketsCount() (int, error) {
	return s.DB.GetTotalTicketsCount()
}
