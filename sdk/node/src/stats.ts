/**
 * Copyright 2016 IBM
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */
/**
 * Licensed Materials - Property of IBM
 * © Copyright IBM Corp. 2016
 */

/*
 * This module provides stats utilities.
 */

/**
 * The Average class keeps a rolling average based on sample values.
 * The sample weight determines how heavily to weight the most recent sample in calculating the current average.
 */
export class Average {

    private avg: number;
    private sampleWeight: number;
    private avgWeight: number;

    constructor() {
        this.setSampleWeight(0.5);
    }

    // Get the average value
    getValue(): number {
        return this.avg;
    }

    /**
     * Add a sample.
     */
    addSample(sample: number) {
        if (this.avg == null) {
            this.avg = sample;
        } else {
            this.avg = (this.avg * this.avgWeight) + (sample * this.sampleWeight);
        }
    }

    /**
     * Get the weight.
     * The weight determines how heavily to weight the most recent sample in calculating the average.
     */
    getSampleWeight(): number {
        return this.sampleWeight;
    }

    /**
     * Set the weight.
     * @params weight A value between 0 and 1.
     */
    setSampleWeight(weight: number):void {
        if ((weight < 0) || (weight > 1)) {
            throw Error("weight must be in range [0,1]; "+weight+" is an invalid value")
        }
        this.sampleWeight = weight;
        this.avgWeight = 1 - weight;
    }

}

/**
 * Class to keep track of an average response time.
 */
export class ResponseTime {

    private avg: Average = new Average();
    private startTime: number;

    constructor() {
    }

    start(): void {
        if (this.startTime != null) {
            throw Error ("started twice without stopping");
        }
        this.startTime = getCurTimeInMs();
    }

    stop(): void {
        if (this.startTime == null) {
            throw Error ("stopped without starting");
        }
        let elapsed = getCurTimeInMs() - this.startTime;
        this.startTime = null;
        this.avg.addSample(elapsed);
    }

    cancel(): void {
        if (this.startTime == null) {
            throw Error ("cancel without starting");
        }
        this.startTime = null;
    }

    // Get the average response time
    getValue(): number {
        return this.avg.getValue();
    }
}

/**
 * Calculate the rate
 */
export class Rate {

    private prevTime: number;
    private avg: Average = new Average();

    constructor() {
        this.avg.setSampleWeight(0.25);
    }

    tick(): void {
        let curTime = getCurTimeInMs();
        if (this.prevTime) {
           let elapsed = curTime - this.prevTime;
           this.avg.addSample(elapsed);
        }
        this.prevTime = curTime;
    }

    // Get the rate in ticks/ms
    getValue(): number {
        return this.avg.getValue();
    }
}

// Get the current time in milliseconds
function getCurTimeInMs(): number {
    return (new Date()).getTime();
}
