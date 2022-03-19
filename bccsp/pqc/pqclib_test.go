package pqc

//func TestBadLibrary(t *testing.T) {
//	_, err := LoadLib("bad")
//	require.Error(t, err)
//	require.Contains(t, err.Error(), "failed to load module")
//}
//
//
//func TestReEntrantLibrary(t *testing.T) {
//	l1, err := LoadLib(libPath)
//	require.NoError(t, err)
//	defer func() { require.NoError(t, CloseLib(l1)) }()
//
//	l2, err := LoadLib(libPath)
//	require.NoError(t, err)
//	defer func() { require.NoError(t, CloseLib(l2)) }()
//}
//
//
//func TestLibraryClosed(t *testing.T) {
//	l, err := LoadLib(libPath)
//	require.NoError(t, err)
//	require.NoError(t, CloseLib(l))
//
//	const expectedMsg = "library closed"
//
//	t.Run("GetSIG", func(t *testing.T) {
//		_, err := GetSign(l, EnabledSigs()[0])
//		require.Error(t, err)
//		assert.Contains(t, err.Error(), expectedMsg)
//	})
//
//	t.Run("Close", func(t *testing.T) {
//		err := CloseLib(l)
//		require.Error(t, err)
//		assert.Contains(t, err.Error(), expectedMsg)
//	})
//}
//
//func TestInvalidSIGAlg(t *testing.T) {
//	l, err := LoadLib(libPath)
//	require.NoError(t, err)
//	defer func() { require.NoError(t, CloseLib(l)) }()
//
//	_, err = GetSign(l, SigType("this will never be valid"))
//	assert.Equal(t, errAlgDisabledOrUnknown, err)
//}
//
//
//func TestLibErr(t *testing.T) {
//	err := libError(operationFailed, "test%d", 123)
//	assert.EqualError(t, err, "test123")
//}
