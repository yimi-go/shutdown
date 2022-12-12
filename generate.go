package shutdown

//go:generate mockgen --source=shutdown.go --destination=shutdown_mock_test.go --package=shutdown
//go:generate mockgen --source=shutdown.go --destination=posixsignal/shutdown_mock_test.go --package=posixsignal
