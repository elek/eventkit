pipeline {
    agent {
       label 'ondemand'
    }

    options {
          timeout(time: 26, unit: 'MINUTES')
    }

    environment {
        GOTRACEBACK = 'all'
    }

    stages {
        stage('Checkout') {
            steps {
               checkout scm
            }
        }
	    stage('Lint') {
    		steps {
    			sh "earthly --ci +lint"
    		}
    	}
    	stage('Test') {
    		steps {
    			sh "earthly --ci +test"
	    	}
	    }
    }
}