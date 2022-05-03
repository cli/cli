package api

func CurrentLoginName(client *Client, hostname string) (string, error) {
	var query struct {
		Viewer struct {
			Login string
		}
	}
	err := client.Query(hostname, "UserCurrent", &query, nil)
	return query.Viewer.Login, err
}

func CurrentUserID(client *Client, hostname string) (string, error) {
	var query struct {
		Viewer struct {
			ID string
		}
	}
	err := client.Query(hostname, "UserCurrent", &query, nil)
	return query.Viewer.ID, err
}
