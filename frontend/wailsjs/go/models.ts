export namespace creds {
	
	export class Account {
	    id: string;
	    name: string;
	    serviceName: string;
	    endpoint: string;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.serviceName = source["serviceName"];
	        this.endpoint = source["endpoint"];
	    }
	}
	export class Input {
	    id: string;
	    name: string;
	    serviceName: string;
	    endpoint: string;
	    appKey: string;
	    appSecret: string;
	    consumerKey: string;
	
	    static createFrom(source: any = {}) {
	        return new Input(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.serviceName = source["serviceName"];
	        this.endpoint = source["endpoint"];
	        this.appKey = source["appKey"];
	        this.appSecret = source["appSecret"];
	        this.consumerKey = source["consumerKey"];
	    }
	}
	export class Status {
	    backend: string;
	    locked: boolean;
	    initialized: boolean;
	    accounts: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.locked = source["locked"];
	        this.initialized = source["initialized"];
	        this.accounts = source["accounts"];
	    }
	}
	export class View {
	    id: string;
	    name: string;
	    serviceName: string;
	    endpoint: string;
	    appKey: string;
	    appSecret: string;
	    consumerKey: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new View(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.serviceName = source["serviceName"];
	        this.endpoint = source["endpoint"];
	        this.appKey = source["appKey"];
	        this.appSecret = source["appSecret"];
	        this.consumerKey = source["consumerKey"];
	        this.active = source["active"];
	    }
	}

}

export namespace ovh {
	
	export class Sender {
	    name: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Sender(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	    }
	}

}

export namespace service {
	
	export class SendInput {
	    message: string;
	    recipients: string;
	    sender: string;
	    sendAt: string;
	    stopClause: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SendInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.recipients = source["recipients"];
	        this.sender = source["sender"];
	        this.sendAt = source["sendAt"];
	        this.stopClause = source["stopClause"];
	    }
	}
	export class SendOutcome {
	    ids: number[];
	    sent: string[];
	    rejected: string[];
	    creditsRemoved: number;
	    scheduled: boolean;
	    analysis: sms.Analysis;
	
	    static createFrom(source: any = {}) {
	        return new SendOutcome(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ids = source["ids"];
	        this.sent = source["sent"];
	        this.rejected = source["rejected"];
	        this.creditsRemoved = source["creditsRemoved"];
	        this.scheduled = source["scheduled"];
	        this.analysis = this.convertValues(source["analysis"], sms.Analysis);
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
	export class ServiceInfo {
	    account: string;
	    serviceName: string;
	    status: string;
	    creditsLeft: number;
	    senders: ovh.Sender[];
	
	    static createFrom(source: any = {}) {
	        return new ServiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account = source["account"];
	        this.serviceName = source["serviceName"];
	        this.status = source["status"];
	        this.creditsLeft = source["creditsLeft"];
	        this.senders = this.convertValues(source["senders"], ovh.Sender);
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

export namespace sms {
	
	export class Analysis {
	    encoding: string;
	    chars: number;
	    units: number;
	    segments: number;
	    perSegment: number;
	    remaining: number;
	
	    static createFrom(source: any = {}) {
	        return new Analysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.encoding = source["encoding"];
	        this.chars = source["chars"];
	        this.units = source["units"];
	        this.segments = source["segments"];
	        this.perSegment = source["perSegment"];
	        this.remaining = source["remaining"];
	    }
	}
	export class Receivers {
	    valid: string[];
	    invalid: string[];
	    dupes: number;
	
	    static createFrom(source: any = {}) {
	        return new Receivers(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.valid = source["valid"];
	        this.invalid = source["invalid"];
	        this.dupes = source["dupes"];
	    }
	}

}

