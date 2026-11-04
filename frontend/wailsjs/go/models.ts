export namespace campaign {
	
	export class RowIssue {
	    line: number;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new RowIssue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.value = source["value"];
	    }
	}

}

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
	    type: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Sender(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.status = source["status"];
	    }
	}

}

export namespace service {
	
	export class CostLine {
	    country: string;
	    recipients: number;
	    creditsPerSms: number;
	    credits: number;
	    priceMin: number;
	    priceMax: number;
	
	    static createFrom(source: any = {}) {
	        return new CostLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.country = source["country"];
	        this.recipients = source["recipients"];
	        this.creditsPerSms = source["creditsPerSms"];
	        this.credits = source["credits"];
	        this.priceMin = source["priceMin"];
	        this.priceMax = source["priceMax"];
	    }
	}
	export class CostEstimate {
	    characters: number;
	    encoding: string;
	    perPart: number;
	    parts: number;
	    recipients: number;
	    lines: CostLine[];
	    totalCredits: number;
	    priceMin: number;
	    priceMax: number;
	    currency: string;
	    unknown: string[];
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new CostEstimate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.characters = source["characters"];
	        this.encoding = source["encoding"];
	        this.perPart = source["perPart"];
	        this.parts = source["parts"];
	        this.recipients = source["recipients"];
	        this.lines = this.convertValues(source["lines"], CostLine);
	        this.totalCredits = source["totalCredits"];
	        this.priceMin = source["priceMin"];
	        this.priceMax = source["priceMax"];
	        this.currency = source["currency"];
	        this.unknown = source["unknown"];
	        this.warning = source["warning"];
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
	export class CostInput {
	    message: string;
	    recipients: string;
	    senderType: string;
	    stopClause: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CostInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.recipients = source["recipients"];
	        this.senderType = source["senderType"];
	        this.stopClause = source["stopClause"];
	    }
	}
	
	export class ImportInfo {
	    id: string;
	    file: string;
	    headers: string[];
	    rows: number;
	    sample: string[][];
	    phoneColumn: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.file = source["file"];
	        this.headers = source["headers"];
	        this.rows = source["rows"];
	        this.sample = source["sample"];
	        this.phoneColumn = source["phoneColumn"];
	    }
	}
	export class LaunchInput {
	    importId: string;
	    phoneColumn: number;
	    template: string;
	    mode: string;
	    stopClause: boolean;
	    senderType: string;
	    rate: number;
	    name: string;
	    sender: string;
	    sendAt: string;
	
	    static createFrom(source: any = {}) {
	        return new LaunchInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.importId = source["importId"];
	        this.phoneColumn = source["phoneColumn"];
	        this.template = source["template"];
	        this.mode = source["mode"];
	        this.stopClause = source["stopClause"];
	        this.senderType = source["senderType"];
	        this.rate = source["rate"];
	        this.name = source["name"];
	        this.sender = source["sender"];
	        this.sendAt = source["sendAt"];
	    }
	}
	export class PlanInput {
	    importId: string;
	    phoneColumn: number;
	    template: string;
	    mode: string;
	    stopClause: boolean;
	    senderType: string;
	    rate: number;
	
	    static createFrom(source: any = {}) {
	        return new PlanInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.importId = source["importId"];
	        this.phoneColumn = source["phoneColumn"];
	        this.template = source["template"];
	        this.mode = source["mode"];
	        this.stopClause = source["stopClause"];
	        this.senderType = source["senderType"];
	        this.rate = source["rate"];
	    }
	}
	export class PlanSample {
	    phone: string;
	    message: string;
	    parts: number;
	    encoding: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanSample(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phone = source["phone"];
	        this.message = source["message"];
	        this.parts = source["parts"];
	        this.encoding = source["encoding"];
	    }
	}
	export class PlanReport {
	    mode: string;
	    recipients: number;
	    invalidCount: number;
	    invalid: campaign.RowIssue[];
	    dupes: number;
	    variables: string[];
	    samples: PlanSample[];
	    totalParts: number;
	    maxParts: number;
	    lines: CostLine[];
	    totalCredits: number;
	    priceMin: number;
	    priceMax: number;
	    currency: string;
	    unknownCountry: number;
	    durationSeconds: number;
	    blocking: string[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new PlanReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.recipients = source["recipients"];
	        this.invalidCount = source["invalidCount"];
	        this.invalid = this.convertValues(source["invalid"], campaign.RowIssue);
	        this.dupes = source["dupes"];
	        this.variables = source["variables"];
	        this.samples = this.convertValues(source["samples"], PlanSample);
	        this.totalParts = source["totalParts"];
	        this.maxParts = source["maxParts"];
	        this.lines = this.convertValues(source["lines"], CostLine);
	        this.totalCredits = source["totalCredits"];
	        this.priceMin = source["priceMin"];
	        this.priceMax = source["priceMax"];
	        this.currency = source["currency"];
	        this.unknownCountry = source["unknownCountry"];
	        this.durationSeconds = source["durationSeconds"];
	        this.blocking = source["blocking"];
	        this.warnings = source["warnings"];
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
	
	export class RecipientsPage {
	    items: store.Recipient[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new RecipientsPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], store.Recipient);
	        this.total = source["total"];
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

export namespace store {
	
	export class Campaign {
	    id: string;
	    name: string;
	    mode: string;
	    accountId: string;
	    template: string;
	    sender: string;
	    stopClause: boolean;
	    // Go type: time
	    sendAt?: any;
	    rate: number;
	    status: string;
	    batchId: string;
	    total: number;
	    sent: number;
	    failed: number;
	    credits: number;
	    error: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Campaign(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.mode = source["mode"];
	        this.accountId = source["accountId"];
	        this.template = source["template"];
	        this.sender = source["sender"];
	        this.stopClause = source["stopClause"];
	        this.sendAt = this.convertValues(source["sendAt"], null);
	        this.rate = source["rate"];
	        this.status = source["status"];
	        this.batchId = source["batchId"];
	        this.total = source["total"];
	        this.sent = source["sent"];
	        this.failed = source["failed"];
	        this.credits = source["credits"];
	        this.error = source["error"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class Recipient {
	    idx: number;
	    phone: string;
	    vars: Record<string, string>;
	    message: string;
	    status: string;
	    error: string;
	    ovhId: number;
	    credits: number;
	
	    static createFrom(source: any = {}) {
	        return new Recipient(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.idx = source["idx"];
	        this.phone = source["phone"];
	        this.vars = source["vars"];
	        this.message = source["message"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.ovhId = source["ovhId"];
	        this.credits = source["credits"];
	    }
	}

}

