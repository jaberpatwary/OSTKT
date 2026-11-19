# Jenkins CI/CD

The `Jenkinsfile` runs the Go test suite, `go vet`, and a Linux amd64 production build on every branch. On `main`, after those stages pass, it invokes the Render deploy hook so Render deploys the checked-in commit.

## Jenkins setup

1. Use a Jenkins agent with Go 1.22 or newer, Git, and `curl` installed.
2. Create a **Secret text** credential with ID `render-deploy-hook` and value set to this Render service's deploy hook URL.
3. Create a Multibranch Pipeline (recommended) or Pipeline job that checks out this repository and loads the root `Jenkinsfile`.
4. Keep production settings such as `DATABASE_URL`, `APP_USER`, `APP_PASS`, `GROQ_API_KEY`, and SMTP credentials in the Render service environment. Do not add them to Jenkinsfile or source control.
5. Configure Render to deploy from the `main` branch and turn off its automatic deploy trigger, so Jenkins is the single deploy trigger. The Jenkins deploy stage is restricted to `main` and runs only after CI succeeds.

## Run Jenkins with Docker Compose

The repository includes a Jenkins container with Go and the Pipeline, Git, and Credentials Binding plugins. Start it with:

```bash
docker compose up --build -d jenkins
docker compose ps jenkins
```

Open `http://localhost:18081`. To get the first-run unlock password, run `docker compose exec jenkins cat /var/jenkins_home/secrets/initialAdminPassword`. Jenkins configuration persists in the `jenkins-home` volume.

The Deploy stage requires the Jenkins credential above. Other branches run CI and publish the build artifact without deploying.
