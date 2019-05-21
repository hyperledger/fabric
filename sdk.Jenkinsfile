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
        stage ('Run sdk-node') {
        // condition should pass then only next step would run else it will skip but won't fail.          
            //when { branch 'QA'}            
                steps {
                  dir('go/src/github.com/Vijaypunugubati') {
                    //Run e2e tests on PR from develop branch
                    sh label: 'Running e2e Tests', script: 'echo "Running the sdk-node Tests"'
                    sh '''
                      echo "B U I L D - F A B R I C"
                      # Print last two commits
                      git log -n2
                      for IMAGES in docker release-clean release docker-thirdparty; do
                        echo "----------> $IMAGES"
                        make -C $BASE_WD/fabric $IMAGES
                      done

                      echo "B U I L D - F A B R I C-CA"
                      rm -rf $BASE_WD/fabric-ca
                      git clone --single-branch -b master https://github.com/hyperledger/fabric-ca $BASE_WD/fabric-ca
                      # Print last two commits
                      git log -n2
                      make $BASE_WD/fabric-ca docker

                      docker pull nexus3.hyperledger.org:10001/hyperledger/fabric-javaenv:amd64-2.0.0-stable
                      docker tag nexus3.hyperledger.org:10001/hyperledger/fabric-javaenv:amd64-2.0.0-stable hyperledger/fabric-javaenv:2.0.0
                      docker tag nexus3.hyperledger.org:10001/hyperledger/fabric-javaenv:amd64-2.0.0-stable hyperledger/fabric-javaenv:amd64-latest
                      
                      docker pull nexus3.hyperledger.org:10001/hyperledger/fabric-nodeenv:amd64-2.0.0-stable
                      docker tag nexus3.hyperledger.org:10001/hyperledger/fabric-nodeenv:amd64-2.0.0-stable hyperledger/fabric-nodeenv:2.0.0
                      docker tag nexus3.hyperledger.org:10001/hyperledger/fabric-nodeenv:amd64-2.0.0-stable hyperledger/fabric-nodeenv:amd64-latest
                      docker images | grep hyperledger

                      echo "S D K - N O D E"
                      echo "STARTING fabric-sdk-node tests"
                      WD="${WORKSPACE}/go/src/github.com/Vijaypunugubati/fabric-sdk-node"
                      rm -rf $WD
                      # Clone fabric-sdk-node repository
                      git clone https://github.com/hyperledger/fabric-sdk-node $WD
                      # Print last two commits
                      git log -n2
                      cd $WD
                      git checkout master


                      # Generate crypto material before running the tests
                      # Run the amd64 gulp task
                      gulp install-and-generate-certs || err_check \
                      "ERROR!!! gulp install and generation of test certificates failed"

                      # Install nvm to install multi node versions
                      wget -qO- https://raw.githubusercontent.com/creationix/nvm/v0.33.11/install.sh | bash
                      export NVM_DIR=$HOME/.nvm
                      source $NVM_DIR/nvm.sh # Setup environment for running nvm
                      node_ver10=10.15.3
                      echo "Intalling Node $node_ver10"
                      nvm install "$node_ver10"
                      nvm use --delete-prefix v$node_ver10 --silent
                      npm install || err_check "npm install failed"
                      npm config set prefix ~/npm
                      npm install -g gulp
                      cd $WD
                      npm install || err_check "npm install failed"
                      npm config set prefix ~/npm
                      npm install -g gulp
                      echo " Run gulp tests"
                      gulp run-end-to-end || err_check "ERROR!!! gulp end-2-end tests failed for node $node_ver10"
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
