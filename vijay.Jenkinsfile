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
        GOROOT="/usr/bin/go1.11.5/go"
        GOPATH="${env.WORKSPACE}/go"
        PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:${env.GOROOT}/bin:${env.GOPATH}/bin"
      }
        
      stages
      {
        stage ('Checkout scm') {
            steps {
                cleanWs deleteDirs: true
                sh label: "Create GOROOT Directory", script: "mkdir -p go${GOVER}"
                sh label: "Create GOPATH Directory", script: "mkdir -p go/src/github.com/Vijaypunugubati/fabric"
                dir('go/src/github.com/Vijaypunugubati/fabric') {
                    checkout scm
                }
            }      
        }
        stage ('Run Tests on Merge') {
        // condition should pass then only next step would run else it will skip but won't fail.
            when { branch 'develop'}          
                steps {
                  dir('go/src/github.com/Vijaypunugubati/fabric') {
                    sh label: 'Running Fabric Unit Tests', script: 'make unit-test'
                    build job: 'code_merge_develop_QA'
                      // sh '''git --version
                      //     git checkout QA
                      //     git merge origin/develop
                      //     git push origin QA''' 
                  }  
                }
        }          
        stage ('Run e2e Tests on QA') {
        // condition should pass then only next step would run else it will skip but won't fail.          
            when { branch 'QA'}            
                steps {
                  dir('go/src/github.com/Vijaypunugubati/fabric') {
                    //Run e2e tests on PR from develop branch
                    sh label: 'Running e2e Tests', script: 'echo "Running the e2e Tests"'
                    build job: 'code_merge_QA_Master'
                      //sh '''git --version
                        //   git checkout master
                        //   git merge origin/QA
                        //   git push origin master'''
                  }
                }  
        }
        stage ('Build and Publish on master') {
        // condition should pass then only next step would run else it will skip but won't fail.            
            when { branch 'master'}            
                steps {
                  dir('go/src/github.com/Vijaypunugubati/fabric') {
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
