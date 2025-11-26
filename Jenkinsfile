pipeline {
    agent any

    environment {
        REGISTRY             = "registry.example.com"
        IMAGE_NAME           = "prometheus-autoscaler-controller"
        IMAGE_REPO           = "${REGISTRY}/${IMAGE_NAME}"

        HELM_CHART_PATH      = "deploy/helm/prometheus-autoscaler"
        HELM_VALUES_FILE     = "${HELM_CHART_PATH}/values.yaml"

        DOCKER_CREDENTIALS   = "docker-reg-creds"
        GIT_CREDENTIALS      = "git-creds"

        GIT_BRANCH           = "main"
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Unit tests') {
            steps {
                sh 'go test ./...'
            }
        }

        stage('Build & Push Image') {
            steps {
                script {
                    // Short SHA for traceability, common GitOps pattern
                    IMAGE_TAG = sh(
                        script: "git rev-parse --short HEAD",
                        returnStdout: true
                    ).trim()

                    docker.withRegistry("https://${REGISTRY}", DOCKER_CREDENTIALS) {
                        def img = docker.build("${IMAGE_REPO}:${IMAGE_TAG}", "-f Dockerfile .")
                        img.push()
                        img.push("latest")
                    }

                    env.IMAGE_TAG = IMAGE_TAG
                }
            }
        }

        stage('Update Helm values (image tag)') {
            steps {
                withCredentials([usernamePassword(
                    credentialsId: GIT_CREDENTIALS,
                    usernameVariable: 'GIT_USER',
                    passwordVariable: 'GIT_PASS'
                )]) {
                    sh '''
                      git config user.email "ci@yourcompany.com"
                      git config user.name "ci-bot"

                      # Ensure we are on main and up to date
                      git fetch origin ${GIT_BRANCH}
                      git checkout ${GIT_BRANCH}
                      git pull origin ${GIT_BRANCH}

                      # Use yq to update image.tag in values.yaml (GitOps-friendly)
                      yq -i ".image.tag = \\"${IMAGE_TAG}\\"" ${HELM_VALUES_FILE}

                      git status
                      git add ${HELM_VALUES_FILE}
                      git commit -m "Update controller image tag to ${IMAGE_TAG}" || echo "No changes to commit"

                      git push https://${GIT_USER}:${GIT_PASS}@${GIT_URL} ${GIT_BRANCH}
                    '''
                }
            }
        }
    }

    post {
        success {
            echo "CI pipeline succeeded. Argo CD will pick up new Helm values and sync."
        }
        failure {
            echo "CI pipeline failed. No changes were pushed to Git."
        }
    }
}
