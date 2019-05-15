pipeline {
    agent any
       triggers {
       githubPullRequests events: [Open()], spec: '* * * * *', triggerMode: 'CRON'
       }
      tools {
        nodejs 'nodejs-8.11.3'
      }
      options 
      {
      //To Ristrict the number of builds to be visible on jenkins
      // we don't fill up our storage!
      buildDiscarder(logRotator(numToKeepStr:'15', artifactNumToKeepStr: '15'))
      //To Timeout
      timeout(time: 10, unit: 'MINUTES')            
      }
      
      environment {
        ARCH="amd64"
        GOVER="1.11.5"
        GOROOT="/usr/bin/go"
        GOPATH="${env.WORKSPACE}/go"
        PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:${env.GOROOT}/bin:${env.GOPATH}/bin"
        BASE_WD="${WORKSPACE}/go/src/github.com/Vijaypunugubati"
      }
        
      stages
      {
        stage ('Checkout scm') {
            steps {
                cleanWs deleteDirs: true
                sh label: "Create GOROOT Directory", script: "mkdir -p go${GOVER}"
                sh label: "Create GOPATH Directory", script: "mkdir -p go/src/github.com/Vijaypunugubati/fab"
                dir('go/src/github.com/Vijaypunugubati/fab') {
                    checkout scm
                }
            }      
        }
        stage ('Run Tests') {
        // condition should pass then only next step would run else it will skip but won't fail.
            //when { branch 'master'}          
                steps {
                  dir('go/src/github.com/Vijaypunugubati/fab') {
                    sh label: 'Running Fabric Unit Tests', script: 'make unit-tests'
		    sh '''
		        docker images | grep hyperledger
		    '''
                    //build job: 'code_merge_develop_QA' 
                  }  
                }
        }
        stage ('Build and Publish on Master') {
        // condition should pass then only next step would run else it will skip but won't fail.            
            //when { branch 'master'}            
                steps {
                  dir('go/src/github.com/Vijaypunugubati/fab') {
                    //Build artifacts and publish to nexus
                    sh label: 'Build Images', script: 'echo "Building Images"'
                    sh label: 'Publish Images', script: 'echo "Publish Images"'
                  }
                }  
        }
        stage('Archive') {     
      steps {
        echo "Archive Logs"
      }
      post {
        success {
          echo "Success"
        }
        always {
            // Archiving the .log files and ignore if empty
          archiveArtifacts artifacts: '**/*.log', allowEmptyArchive: true
        }
		} //post
      } // stages
    }  
} // pipeline
