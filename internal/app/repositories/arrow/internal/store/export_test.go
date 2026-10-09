package store

func WaitRechecks(s Store) {
	s.(*storeService).rechecking.Wait()
}
