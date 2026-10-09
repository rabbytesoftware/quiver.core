package store

func WaitRefs(s Store) {
	s.(*storeService).running.Wait()
}
