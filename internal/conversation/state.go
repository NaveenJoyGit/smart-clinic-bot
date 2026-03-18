package conversation

import (
	"context"
	"encoding/json"
)

type State string

const (
	StateStart               State = "START"
	StateAnsweringFAQ        State = "ANSWERING_FAQ"
	StateBookingIntent       State = "BOOKING_INTENT"
	StateAskTime             State = "ASK_TIME"
	StateAppointmentCaptured State = "APPOINTMENT_CAPTURED"
)

type ConvData struct {
	State         State  `json:"state"`
	PatientName   string `json:"patient_name"`
	PreferredTime string `json:"preferred_time"`
}

// GetConvData reads state from conversations.metadata.
// Returns default ConvData{State: StateStart} if no row exists.
func (m *Manager) GetConvData(ctx context.Context, clinicID, platform, senderID string) (ConvData, error) {
	var raw []byte
	err := m.pool.QueryRow(ctx,
		`SELECT metadata FROM conversations WHERE clinic_id = $1 AND platform = $2 AND external_id = $3`,
		clinicID, platform, senderID,
	).Scan(&raw)
	if err != nil {
		return ConvData{State: StateStart}, nil // no row yet → default
	}
	var d ConvData
	_ = json.Unmarshal(raw, &d)
	if d.State == "" {
		d.State = StateStart
	}
	return d, nil
}

// SetConvData merges state fields into conversations.metadata.
func (m *Manager) SetConvData(ctx context.Context, clinicID, platform, senderID string, d ConvData) error {
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = m.pool.Exec(ctx,
		`UPDATE conversations SET metadata = metadata || $4::jsonb
         WHERE clinic_id = $1 AND platform = $2 AND external_id = $3`,
		clinicID, platform, senderID, raw,
	)
	return err
}
