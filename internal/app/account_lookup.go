package app

func (s *Server) accountByToken(token string) Account {
	for _, account := range s.store.LoadAccounts() {
		if account.AccessToken == token {
			return account
		}
	}
	return Account{AccessToken: token}
}
