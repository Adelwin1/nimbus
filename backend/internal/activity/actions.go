package activity

const (
	ActionUserRegistered     = "user.registered"
	ActionUserLoggedIn       = "user.logged_in"
	ActionUserTokenRefreshed = "user.token_refreshed"
	ActionUserLoggedOut      = "user.logged_out"

	ActionApplicationCreated = "application.created"
	ActionApplicationUpdated = "application.updated"
	ActionApplicationDeleted = "application.deleted"

	ActionHealthCheckRequested = "health_check.requested"

	ActionDeploymentCreated   = "deployment.created"
	ActionDeploymentSucceeded = "deployment.succeeded"
	ActionDeploymentFailed    = "deployment.failed"

	ActionIncidentOpened       = "incident.opened"
	ActionIncidentAcknowledged = "incident.acknowledged"
	ActionIncidentResolved     = "incident.resolved"

	ActionRollbackStarted   = "rollback.started"
	ActionRollbackSucceeded = "rollback.succeeded"
	ActionRollbackFailed    = "rollback.failed"
)

const (
	EntityUser        = "user"
	EntityApplication = "application"
	EntityHealthCheck = "health_check"
	EntityDeployment  = "deployment"
	EntityIncident    = "incident"
	EntityRollback    = "rollback"
)
