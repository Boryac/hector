export namespace main {
	
	export class BackendStatus {
	    running: boolean;
	    pid?: number;
	    healthy: boolean;
	    endpoint?: string;
	    version?: string;
	    startedFor?: string;
	
	    static createFrom(source: any = {}) {
	        return new BackendStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.pid = source["pid"];
	        this.healthy = source["healthy"];
	        this.endpoint = source["endpoint"];
	        this.version = source["version"];
	        this.startedFor = source["startedFor"];
	    }
	}
	export class InitState {
	    initialized: boolean;
	    defaultDataDir: string;
	    dataDir: string;
	    addr: string;
	    pgConfigured: boolean;
	    autostart: boolean;
	    exitPending: boolean;
	    backendStatus?: BackendStatus;
	
	    static createFrom(source: any = {}) {
	        return new InitState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.initialized = source["initialized"];
	        this.defaultDataDir = source["defaultDataDir"];
	        this.dataDir = source["dataDir"];
	        this.addr = source["addr"];
	        this.pgConfigured = source["pgConfigured"];
	        this.autostart = source["autostart"];
	        this.exitPending = source["exitPending"];
	        this.backendStatus = this.convertValues(source["backendStatus"], BackendStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

