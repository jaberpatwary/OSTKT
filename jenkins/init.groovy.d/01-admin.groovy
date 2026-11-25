import hudson.model.User
import hudson.security.FullControlOnceLoggedInAuthorizationStrategy
import hudson.security.HudsonPrivateSecurityRealm
import jenkins.model.Jenkins

def adminId = System.getenv('JENKINS_ADMIN_ID')?.trim()
def adminPassword = System.getenv('JENKINS_ADMIN_PASSWORD')
if (!adminId || !adminPassword) {
    throw new IllegalStateException('JENKINS_ADMIN_ID and JENKINS_ADMIN_PASSWORD must be configured')
}

def jenkins = Jenkins.get()
def realm = jenkins.getSecurityRealm()
if (!(realm instanceof HudsonPrivateSecurityRealm)) {
    realm = new HudsonPrivateSecurityRealm(false)
    jenkins.setSecurityRealm(realm)
}

def admin = User.getById(adminId, false)
if (admin == null) {
    realm.createAccount(adminId, adminPassword)
} else {
    admin.addProperty(HudsonPrivateSecurityRealm.Details.fromPlainPassword(adminPassword))
}

jenkins.setAuthorizationStrategy(new FullControlOnceLoggedInAuthorizationStrategy())
jenkins.save()

// Keep this as a one-time bootstrap; later restarts must not reset a changed password.
new File(jenkins.getRootDir(), 'init.groovy.d/01-admin.groovy').delete()
