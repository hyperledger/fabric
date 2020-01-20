pipeline {
  agent any
  stages {
    stage('git pull') {
      steps {
        sh '''git pull
'''
        sh 'git checkout $(tag)'
      }
    }

  }
}