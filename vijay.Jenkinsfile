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
                    //build job: 'code_merge_develop_QA' 
                  }  
                }
        }          
        stage ('Run e2e Tests') {
        // condition should pass then only next step would run else it will skip but won't fail.          
            //when { branch 'QA'}            
                steps {
                  dir('go/src/github.com/Vijaypunugubati') {
                    //Run e2e tests on PR from develop branch
                    sh label: 'Running e2e Tests', script: 'echo "Running the e2e Tests"'
                    sh '''
                      echo "B U I L D - F A B R I C"
                      # Print last two commits
                      git -C $BASE_WD/fabric log -n2
                      for IMAGES in docker release-clean release docker-thirdparty; do
                        echo -e "\033[1m----------> $IMAGES\033[0m"
                        make -C $BASE_WD/fabric $IMAGES
                      done

                      echo "B U I L D - F A B R I C-CA"
                      rm -rf $BASE_WD/fabric-ca
                      git clone --single-branch -b master https://github.com/hyperledger/fabric-ca $BASE_WD/fabric-ca
                      # Print last two commits
                      git -C $BASE_WD/fabric-ca log -n2
                      make -C $BASE_WD/fabric-ca docker
                      docker pull nexus3.hyperledger.org:10001/hyperledger/fabric-javaenv:amd64-2.0.0-stable
                      docker tag nexus3.hyperledger.org:10001/hyperledger/fabric-javaenv:amd64-2.0.0-stable hyperledger/$REPO-javaenv:2.0.0
                      docker tag nexus3.hyperledger.org:10001/hyperledger/fabric-javaenv:amd64-2.0.0-stable hyperledger/$REPO-javaenv:amd64-latest
                      docker images | grep hyperledger

                      echo "S D K - J A V A"
                      echo "STARTING fabric-sdk-java tests"
                      WD="${WORKSPACE}/go/src/github.com/Vijaypunugubati/fabric-sdk-java"
                      rm -rf $WD
                      # Clone fabric-sdk-java repository
                      git clone https://github.com/hyperledger/fabric-sdk-java $WD
                      cd $WD
                      git checkout master
                      export GOPATH=$WD/src/test/fixture
                      cd $WD/src/test
                      chmod +x cirun.sh
                      source cirun.sh
                    '''
                    //build job: 'code_merge_QA_Master'
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
