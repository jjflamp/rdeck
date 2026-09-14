export namespace app {
	
	export class BulkPreview {
	    keys: redisclient.KeyInfo[];
	    total: number;
	    complete: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BulkPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.keys = this.convertValues(source["keys"], redisclient.KeyInfo);
	        this.total = source["total"];
	        this.complete = source["complete"];
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
	export class BulkRequest {
	    op: string;
	    conn_id: string;
	    db: number;
	    pattern: string;
	    target_conn_id?: string;
	    target_db?: number;
	    ttl?: number;
	    rdb_path?: string;
	    include?: string[];
	
	    static createFrom(source: any = {}) {
	        return new BulkRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.op = source["op"];
	        this.conn_id = source["conn_id"];
	        this.db = source["db"];
	        this.pattern = source["pattern"];
	        this.target_conn_id = source["target_conn_id"];
	        this.target_db = source["target_db"];
	        this.ttl = source["ttl"];
	        this.rdb_path = source["rdb_path"];
	        this.include = source["include"];
	    }
	}
	export class ConnSummary {
	    id: string;
	    mode: string;
	    version: string;
	    address: string;
	    databases: redisclient.DBInfo[];
	
	    static createFrom(source: any = {}) {
	        return new ConnSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.mode = source["mode"];
	        this.version = source["version"];
	        this.address = source["address"];
	        this.databases = this.convertValues(source["databases"], redisclient.DBInfo);
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
	export class FormatterInfo {
	    name: string;
	    description: string;
	    read_only: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FormatterInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.read_only = source["read_only"];
	    }
	}
	export class TestResult {
	    ok: boolean;
	    message?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.error = source["error"];
	    }
	}
	export class TreeResult {
	    nodes: tree.Node[];
	    total: number;
	    complete: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TreeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodes = this.convertValues(source["nodes"], tree.Node);
	        this.total = source["total"];
	        this.complete = source["complete"];
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

export namespace formatter {
	
	export class ExtFormatterInfo {
	    id: string;
	    name: string;
	    "read-only": boolean;
	
	    static createFrom(source: any = {}) {
	        return new ExtFormatterInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this["read-only"] = source["read-only"];
	    }
	}
	export class Result {
	    output: string;
	    format: string;
	    read_only: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.output = source["output"];
	        this.format = source["format"];
	        this.read_only = source["read_only"];
	        this.error = source["error"];
	    }
	}

}

export namespace keymodel {
	
	export class EditRequest {
	    op: string;
	    type: string;
	    key?: string;
	    newKey?: string;
	    index?: number;
	    value: string;
	    score?: number;
	
	    static createFrom(source: any = {}) {
	        return new EditRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.op = source["op"];
	        this.type = source["type"];
	        this.key = source["key"];
	        this.newKey = source["newKey"];
	        this.index = source["index"];
	        this.value = source["value"];
	        this.score = source["score"];
	    }
	}
	export class KeyMeta {
	    key: string;
	    type: string;
	    ttl: number;
	    rows: number;
	    size: number;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KeyMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.type = source["type"];
	        this.ttl = source["ttl"];
	        this.rows = source["rows"];
	        this.size = source["size"];
	        this.truncated = source["truncated"];
	    }
	}
	export class Row {
	    index: number;
	    key?: string;
	    value: string;
	    score?: number;
	
	    static createFrom(source: any = {}) {
	        return new Row(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.key = source["key"];
	        this.value = source["value"];
	        this.score = source["score"];
	    }
	}
	export class RowsPage {
	    rows: Row[];
	    nextCursor: string;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new RowsPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = this.convertValues(source["rows"], Row);
	        this.nextCursor = source["nextCursor"];
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

}

export namespace redisclient {
	
	export class Config {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    auth?: string;
	    username?: string;
	    timeout_connect?: number;
	    timeout_execute?: number;
	    ssl?: boolean;
	    ssl_ca_cert_path?: string;
	    ssl_private_key_path?: string;
	    ssl_local_cert_path?: string;
	    ssl_ignore_all_errors?: boolean;
	    use_ssh_tunnel?: boolean;
	    ssh_host?: string;
	    ssh_port?: number;
	    ssh_user?: string;
	    ssh_password?: string;
	    ssh_private_key_path?: string;
	    ssh_public_key_path?: string;
	    ssh_agent?: boolean;
	    ssh_agent_path?: string;
	    ask_ssh_password?: boolean;
	    cluster_host_override?: boolean;
	    keys_pattern?: string;
	    namespace_separator?: string;
	    db_scan_limit?: number;
	    default_formatter?: string;
	    icon_color?: string;
	    filter_history?: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.auth = source["auth"];
	        this.username = source["username"];
	        this.timeout_connect = source["timeout_connect"];
	        this.timeout_execute = source["timeout_execute"];
	        this.ssl = source["ssl"];
	        this.ssl_ca_cert_path = source["ssl_ca_cert_path"];
	        this.ssl_private_key_path = source["ssl_private_key_path"];
	        this.ssl_local_cert_path = source["ssl_local_cert_path"];
	        this.ssl_ignore_all_errors = source["ssl_ignore_all_errors"];
	        this.use_ssh_tunnel = source["use_ssh_tunnel"];
	        this.ssh_host = source["ssh_host"];
	        this.ssh_port = source["ssh_port"];
	        this.ssh_user = source["ssh_user"];
	        this.ssh_password = source["ssh_password"];
	        this.ssh_private_key_path = source["ssh_private_key_path"];
	        this.ssh_public_key_path = source["ssh_public_key_path"];
	        this.ssh_agent = source["ssh_agent"];
	        this.ssh_agent_path = source["ssh_agent_path"];
	        this.ask_ssh_password = source["ask_ssh_password"];
	        this.cluster_host_override = source["cluster_host_override"];
	        this.keys_pattern = source["keys_pattern"];
	        this.namespace_separator = source["namespace_separator"];
	        this.db_scan_limit = source["db_scan_limit"];
	        this.default_formatter = source["default_formatter"];
	        this.icon_color = source["icon_color"];
	        this.filter_history = source["filter_history"];
	    }
	}
	export class DBInfo {
	    index: number;
	    keys: number;
	    label?: string;
	
	    static createFrom(source: any = {}) {
	        return new DBInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.keys = source["keys"];
	        this.label = source["label"];
	    }
	}
	export class KeyInfo {
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new KeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}

}

export namespace serverstats {
	
	export class ClientRow {
	    id: string;
	    addr: string;
	    name: string;
	    age: string;
	    idle: string;
	    db: string;
	    cmd: string;
	
	    static createFrom(source: any = {}) {
	        return new ClientRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.addr = source["addr"];
	        this.name = source["name"];
	        this.age = source["age"];
	        this.idle = source["idle"];
	        this.db = source["db"];
	        this.cmd = source["cmd"];
	    }
	}
	export class InfoSnapshot {
	    sections: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new InfoSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sections = source["sections"];
	    }
	}
	export class SlowLogEntry {
	    id: number;
	    timestamp: number;
	    duration_us: number;
	    command: string;
	    client: string;
	
	    static createFrom(source: any = {}) {
	        return new SlowLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.timestamp = source["timestamp"];
	        this.duration_us = source["duration_us"];
	        this.command = source["command"];
	        this.client = source["client"];
	    }
	}

}

export namespace settings {
	
	export class Settings {
	    value_size_limit: number;
	    app_font?: string;
	    app_font_size?: number;
	    value_editor_font?: string;
	    scan_limit: number;
	    ext_server_url?: string;
	    locale?: string;
	    dark_mode: boolean;
	    use_system_proxy: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value_size_limit = source["value_size_limit"];
	        this.app_font = source["app_font"];
	        this.app_font_size = source["app_font_size"];
	        this.value_editor_font = source["value_editor_font"];
	        this.scan_limit = source["scan_limit"];
	        this.ext_server_url = source["ext_server_url"];
	        this.locale = source["locale"];
	        this.dark_mode = source["dark_mode"];
	        this.use_system_proxy = source["use_system_proxy"];
	    }
	}

}

export namespace tree {
	
	export class Node {
	    name: string;
	    fullPath: string;
	    isNamespace: boolean;
	    keyType?: string;
	    count?: number;
	    children?: Node[];
	
	    static createFrom(source: any = {}) {
	        return new Node(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.fullPath = source["fullPath"];
	        this.isNamespace = source["isNamespace"];
	        this.keyType = source["keyType"];
	        this.count = source["count"];
	        this.children = this.convertValues(source["children"], Node);
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

