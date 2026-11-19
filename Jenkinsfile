pipeline {
    agent any

    options {
        timestamps()
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '20'))
    }

    environment {
        GOFLAGS = '-mod=readonly'
        CGO_ENABLED = '0'
    }

    stages {
        stage('Check toolchain') {
            steps {
                sh 'go version'
            }
        }

        stage('Test') {
            steps {
                sh 'go test ./...'
            }
        }

        stage('Vet') {
            steps {
                sh 'go vet ./...'
            }
        }

        stage('Build Linux artifact') {
            steps {
                sh '''
                    mkdir -p dist
                    GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/auto-ticket-system .
                '''
                archiveArtifacts artifacts: 'dist/auto-ticket-system', fingerprint: true
            }
        }

        stage('Deploy to Render') {
            when {
                expression {
                    env.BRANCH_NAME == 'main' || env.GIT_BRANCH == 'main' || env.GIT_BRANCH == 'origin/main'
                }
            }
            steps {
                withCredentials([string(credentialsId: 'render-deploy-hook', variable: 'RENDER_DEPLOY_HOOK')]) {
                    sh '''
                        test -n "$RENDER_DEPLOY_HOOK"
                        printf 'url = "%s"\nrequest = "POST"\n' "$RENDER_DEPLOY_HOOK" | curl --config - --fail --silent --show-error --output /dev/null
                    '''
                }
            }
        }
    }

    post {
        always {
            deleteDir()
        }
    }
}
