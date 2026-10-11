package dnstask

import "encoding/json"

// Keep completed checks only; no credentials or raw provider output are stored.
func (s *Service) saveResult(id string, state State) error {
	state.Running, state.Checking = false, false
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tx, err := s.store.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO dns_history(task_id,body) VALUES(?,?)", id, string(body)); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM dns_history WHERE task_id=? AND seq NOT IN (SELECT seq FROM dns_history WHERE task_id=? ORDER BY seq DESC LIMIT 100)", id, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) history(id string, limit int) ([]State, error) {
	rows, err := s.store.DB.Query("SELECT body FROM dns_history WHERE task_id=? ORDER BY seq DESC LIMIT ?", id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []State{}
	for rows.Next() {
		var body string
		var state State
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(body), &state); err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, rows.Err()
}
