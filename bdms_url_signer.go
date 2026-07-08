package douyinLive

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/dop251/goja"
	"github.com/jwwsjlm/douyinLive/v2/utils"
)

//go:embed jsScript/bdms.js
var bdmsJS string

// BDMSURLSignResult 闁哄嫷鍨卞﹢浼村捶?BDMS 缂佹稒鍎抽幃鏇㈠闯閵娿劎绠查柛銉у仧濞?URL 缂佹稒鍎抽幃鏇犵磼閹惧浜柕?// BDMSURLSignResult contains the locally signed webcast URL and safe diagnostics.
type BDMSURLSignResult struct {
	SignedURL         string         `json:"signedUrl"`
	SignedURLRedacted string         `json:"signedUrlRedacted"`
	Lengths           map[string]int `json:"lengths"`
}

// signWebcastURL 濞达綀娉曢弫?Goja 闁告劕鎳庣粊?bdms.js 缂?/webcast/* URL 閻炴稏鍎电紞?msToken 濞?a_bogus闁?// signWebcastURL signs /webcast/* URLs with the embedded bdms.js Goja runtime.
func (dl *DouyinLive) signWebcastURL(ctx context.Context, unsignedURL string, msToken string) (*BDMSURLSignResult, error) {
	if dl == nil {
		return nil, errors.New("nil DouyinLive")
	}
	return signURLWithLocalBDMS(ctx, unsignedURL, dl.getCookieString(), msToken, dl.userAgent)
}

func signURLWithLocalBDMS(ctx context.Context, unsignedURL string, cookie string, msToken string, userAgent string) (*BDMSURLSignResult, error) {
	unsignedURL = strings.TrimSpace(unsignedURL)
	if unsignedURL == "" {
		return nil, errors.New("unsigned url is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	hasProvidedMsToken := urlHasQueryKey(unsignedURL, "msToken")
	externalMsToken := firstNonEmptyBDMSString(strings.TrimSpace(msToken), pickCookieValueForBDMS(cookie, "msToken"))
	canRegenerateMsToken := !hasProvidedMsToken && externalMsToken == ""
	maxAttempts := 1
	if canRegenerateMsToken {
		maxAttempts = 12
	}

	lastSignedURL := unsignedURL
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		candidateURL := ensureBDMSMsTokenInURL(unsignedURL, cookie, externalMsToken)
		signedURL, err := signURLWithGojaBDMS(candidateURL, cookie, userAgent)
		if err != nil {
			lastErr = err
			continue
		}
		lastSignedURL = signedURL
		if !canRegenerateMsToken || queryValueLength(lastSignedURL, "a_bogus") == 188 {
			break
		}
	}
	if lastSignedURL == "" || lastSignedURL == unsignedURL && lastErr != nil {
		return nil, lastErr
	}

	result := &BDMSURLSignResult{
		SignedURL:         lastSignedURL,
		SignedURLRedacted: redactSignedURLForLog(lastSignedURL),
		Lengths:           queryParamLengths(lastSignedURL, "msToken", "a_bogus", "X-Bogus", "_signature"),
	}
	if result.SignedURL == "" {
		return nil, errors.New("bdms signer returned empty signed url")
	}
	return result, nil
}

func signURLWithGojaBDMS(unsignedURL string, cookie string, userAgent string) (string, error) {
	vm := goja.New()
	if err := installGojaBDMSEnvironment(vm, cookie, userAgent); err != nil {
		return "", err
	}
	if _, err := vm.RunString(bdmsJS); err != nil {
		return "", fmt.Errorf("load bdms.js into goja failed: %w", err)
	}
	unsignedJSON, _ := json.Marshal(unsignedURL)
	script := `
		(function () {
			var candidateUrl = ` + string(unsignedJSON) + `;
			if (!window.bdms || typeof window.bdms.init !== "function") {
				throw new Error("window.bdms.init is not available");
			}
			window.__bdmsCalls = [];
			window.bdms.init({
				aid: 6383,
				paths: ["/webcast/room/web/enter", "/webcast/im/fetch"],
				pageId: 1
			});
			var xhr = new window.XMLHttpRequest();
			xhr.open("GET", candidateUrl, true);
			xhr.send();
			for (var i = window.__bdmsCalls.length - 1; i >= 0; i--) {
				if (window.__bdmsCalls[i].kind === "open") return String(window.__bdmsCalls[i].url || "");
			}
			return candidateUrl;
		})()
	`
	value, err := vm.RunString(script)
	if err != nil {
		return "", fmt.Errorf("execute bdms goja signer failed: %w", err)
	}
	signedURL := strings.TrimSpace(value.String())
	if signedURL == "" {
		return "", errors.New("bdms goja signer returned empty url")
	}
	return signedURL, nil
}

func installGojaBDMSEnvironment(vm *goja.Runtime, cookie string, userAgent string) error {
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	}
	uaJSON, _ := json.Marshal(userAgent)
	cookieJSON, _ := json.Marshal(cookie)
	env := `
(function(){
 var root = globalThis;
 var ua = ` + string(uaJSON) + `;
 var initialCookie = ` + string(cookieJSON) + `;
 function defineValue(obj,key,value){ try{ Object.defineProperty(obj,key,{configurable:true,value:value}); }catch(_){ try{ obj[key]=value; }catch(__){} } }
 function makeStorage(seed){ var data={}; Object.keys(seed||{}).forEach(function(k){data[k]=String(seed[k]);}); return { get length(){return Object.keys(data).length;}, key:function(i){return Object.keys(data)[i]||null;}, getItem:function(k){k=String(k); return Object.prototype.hasOwnProperty.call(data,k)?data[k]:null;}, setItem:function(k,v){data[String(k)]=String(v);}, removeItem:function(k){delete data[String(k)];}, clear:function(){data={};} }; }
 function makeThenable(value){ return { then:function(resolve){ if(typeof resolve==='function') resolve(value); return makeThenable(value); }, catch:function(){ return makeThenable(value); } }; }
 function makeCookieJar(header){ var jar={}; String(header||'').split(';').forEach(function(part){ part=part.trim(); if(!part) return; var i=part.indexOf('='); if(i<=0) return; jar[part.slice(0,i).trim()]=part.slice(i+1).trim(); }); return { get:function(){ return Object.keys(jar).map(function(k){ return k+'='+jar[k]; }).join('; '); }, set:function(value){ var parts=String(value||'').split(';'); var pair=(parts.shift()||'').trim(); var i=pair.indexOf('='); if(i<=0) return; jar[pair.slice(0,i).trim()]=pair.slice(i+1).trim(); } }; }
 function encodeQuery(s){ return encodeURIComponent(String(s)).replace(/%20/g,'+'); }
 function decodeQuery(s){ try{return decodeURIComponent(String(s).replace(/\+/g,'%20'));}catch(_){return String(s);} }
 function URLSearchParams(init){ this._pairs=[]; if(init instanceof URLSearchParams){ for(var i=0;i<init._pairs.length;i++) this._pairs.push([init._pairs[i][0],init._pairs[i][1]]); return; } var q=String(init||''); if(q.charAt(0)==='?') q=q.slice(1); if(!q) return; var parts=q.split('&'); for(var j=0;j<parts.length;j++){ if(parts[j]==='') continue; var eq=parts[j].indexOf('='); var k=eq<0?parts[j]:parts[j].slice(0,eq); var v=eq<0?'':parts[j].slice(eq+1); this._pairs.push([decodeQuery(k), decodeQuery(v)]); } }
 URLSearchParams.prototype.append=function(k,v){this._pairs.push([String(k),String(v)]);};
 URLSearchParams.prototype.delete=function(k){k=String(k); this._pairs=this._pairs.filter(function(p){return p[0]!==k;});};
 URLSearchParams.prototype.get=function(k){k=String(k); for(var i=0;i<this._pairs.length;i++) if(this._pairs[i][0]===k) return this._pairs[i][1]; return null;};
 URLSearchParams.prototype.has=function(k){return this.get(k)!==null;};
 URLSearchParams.prototype.set=function(k,v){k=String(k); this.delete(k); this.append(k,v);};
 URLSearchParams.prototype.forEach=function(cb,thisArg){ for(var i=0;i<this._pairs.length;i++) cb.call(thisArg,this._pairs[i][1],this._pairs[i][0],this); };
 URLSearchParams.prototype.toString=function(){ return this._pairs.map(function(p){ return encodeQuery(p[0])+'='+encodeQuery(p[1]); }).join('&'); };
 URLSearchParams.prototype.entries=function(){ var a=this._pairs.slice(), i=0; return { next:function(){ return i<a.length ? {value:a[i++],done:false} : {done:true}; }, [Symbol.iterator]:function(){return this;} }; };
 URLSearchParams.prototype[Symbol.iterator]=URLSearchParams.prototype.entries;
 function URL(input, base){ var raw=String(input||''); if(base && !/^https?:\/\//i.test(raw)){ raw=String(base).replace(/[#?].*$/,'').replace(/\/$/,'')+'/'+raw.replace(/^\//,''); } var m=raw.match(/^([a-zA-Z][a-zA-Z0-9+.-]*:)?\/\/([^\/?#]*)([^?#]*)(\?[^#]*)?(#.*)?$/); if(!m){ m=['', 'https:', 'live.douyin.com', raw.split('?')[0]||'/', raw.indexOf('?')>=0?'?'+raw.split('?').slice(1).join('?').split('#')[0]:'', raw.indexOf('#')>=0?'#'+raw.split('#').slice(1).join('#'):'']; }
   this.protocol=m[1]||'https:'; this.host=m[2]||'live.douyin.com'; this.hostname=this.host.split(':')[0]; this.port=(this.host.split(':')[1]||''); this.pathname=m[3]||'/'; this.hash=m[5]||''; this.searchParams=new URLSearchParams(m[4]||''); this.origin=this.protocol+'//'+this.host; this._sync=function(){ var q=this.searchParams.toString(); this.search=q?'?'+q:''; this.href=this.origin+this.pathname+this.search+this.hash; }; this._sync(); }
 URL.prototype.toString=function(){ this._sync(); return this.href; };
 URL.prototype.toJSON=function(){ return this.toString(); };
 Object.defineProperty(URL.prototype,'search',{get:function(){ var q=this.searchParams.toString(); return q?'?'+q:'';}, set:function(v){this.searchParams=new URLSearchParams(v);}});
 root.URL = URL; root.URLSearchParams = URLSearchParams;
 function patchTypedArrayFrom(Ctor){ if(!Ctor) return; var nativeFrom=Ctor.from; Ctor.from=function(source, mapFn, thisArg){ if(typeof source==='string'){ var arr=[]; for(var i=0;i<source.length;i++){ var v=source.charCodeAt(i); arr.push(typeof mapFn==='function'?mapFn.call(thisArg, source.charAt(i), i):v); } return new Ctor(arr); } try { return nativeFrom.call(Ctor, source, mapFn, thisArg); } catch(e) { if(source && typeof source.length==='number'){ var out=[]; for(var j=0;j<source.length;j++){ var val=source[j]; out.push(typeof mapFn==='function'?mapFn.call(thisArg, val, j):val); } return new Ctor(out); } throw e; } }; }
 ['Uint8Array','Uint8ClampedArray','Int8Array','Uint16Array','Int16Array','Uint32Array','Int32Array'].forEach(function(name){ patchTypedArrayFrom(root[name]); });
 root.window=root; root.self=root; root.top=root; root.parent=root; root.globalThis=root;
 root.location = new URL('https://live.douyin.com/shenyuey');
 root.document = { referrer:'https://live.douyin.com/shenyuey', visibilityState:'visible', hidden:false, compatMode:'CSS1Compat', readyState:'complete', documentElement:null, head:null, body:null, addEventListener:function(){}, removeEventListener:function(){}, getElementsByTagName:function(){return[];}, createEvent:function(){return{initEvent:function(){}};} };
 var cookieJar = makeCookieJar(initialCookie); Object.defineProperty(root.document,'cookie',{get:function(){return cookieJar.get();}, set:function(v){cookieJar.set(v);}});
 function makeElement(tag){ return { tagName:String(tag||'').toUpperCase(), style:{}, children:[], width:300, height:150, appendChild:function(c){this.children.push(c);return c;}, removeChild:function(c){return c;}, setAttribute:function(k,v){this[k]=String(v);}, getAttribute:function(k){return this[k]||null;}, addEventListener:function(){}, removeEventListener:function(){}, getBoundingClientRect:function(){return{left:0,top:0,width:this.width||0,height:this.height||0};}, getContext:function(){return null;}, toDataURL:function(){return 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ';} }; }
 root.document.createElement=makeElement; root.document.documentElement=makeElement('html'); root.document.head=makeElement('head'); root.document.body=makeElement('body');
 root.navigator={ userAgent:ua, appCodeName:'Mozilla', appName:'Netscape', appVersion:ua.replace(/^Mozilla\//,''), platform:'Win32', product:'Gecko', productSub:'20030107', vendor:'Google Inc.', vendorSub:'', language:'zh-CN', languages:['zh-CN','zh'], cookieEnabled:true, onLine:true, doNotTrack:null, deviceMemory:32, hardwareConcurrency:20, maxTouchPoints:0, webdriver:false, pdfViewerEnabled:true, plugins:[{name:'PDF Viewer',filename:'internal-pdf-viewer',description:'Portable Document Format'},{name:'Chrome PDF Viewer',filename:'internal-pdf-viewer',description:'Portable Document Format'}], mimeTypes:[{type:'application/pdf'}], sendBeacon:function(){return true;}, vibrate:function(){return true;}, getBattery:function(){return makeThenable({charging:true,chargingTime:0,dischargingTime:Infinity,level:1});} };
 root.screen={width:1920,height:1080,availWidth:1920,availHeight:1032,colorDepth:24,pixelDepth:24}; root.devicePixelRatio=1; root.innerWidth=300; root.innerHeight=150; root.outerWidth=945; root.outerHeight=1012; root.screenX=0; root.screenY=0; root.pageXOffset=0; root.pageYOffset=0;
 root.localStorage=makeStorage({'__msuuid__':'00000000-0000-4000-8000-000000000000'}); root.sessionStorage=makeStorage({'sessionStarted':'1'}); root.indexedDB={};
 root.chrome={runtime:{}, loadTimes:function(){}, csi:function(){}}; root.Image=function(){return makeElement('img');}; root.TouchEvent=function(){}; root.RTCPeerConnection=function(){return{createDataChannel:function(){return{};},createOffer:function(){return makeThenable({sdp:''});},setLocalDescription:function(){return makeThenable(undefined);},close:function(){},addEventListener:function(){},removeEventListener:function(){}};}; root.webkitRTCPeerConnection=root.RTCPeerConnection;
 root.crypto=root.crypto||{}; root.crypto.getRandomValues=root.crypto.getRandomValues||function(a){ for(var i=0;i<a.length;i++) a[i]=(i*17+29)&255; return a; };
 root.atob=function(s){ var chars='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/='; var str=String(s).replace(/=+$/,''); var output=''; if(str.length%4==1) throw new Error('InvalidCharacterError'); for(var bc=0,bs=0,buffer,idx=0; buffer=str.charAt(idx++); ~buffer && (bs=bc%4?bs*64+buffer:buffer, bc++%4) ? output+=String.fromCharCode(255&bs>>(-2*bc&6)) : 0) buffer=chars.indexOf(buffer); return output; };
 root.btoa=function(input){ var chars='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/='; var str=String(input); var output=''; for(var block=0,charCode,i=0,map=chars; str.charAt(i|0) || (map='=', i%1); output+=map.charAt(63&block>>8-i%1*8)){ charCode=str.charCodeAt(i+=3/4); if(charCode>0xFF) throw new Error('InvalidCharacterError'); block=block<<8|charCode; } return output; };
 root.addEventListener=function(){}; root.removeEventListener=function(){}; root.setTimeout=function(fn){ if(typeof fn==='function') fn(); return 1;}; root.clearTimeout=function(){}; root.setInterval=function(){return 1;}; root.clearInterval=function(){}; root.requestAnimationFrame=function(fn){ if(typeof fn==='function') fn(Date.now()); return 1;}; root.cancelAnimationFrame=function(){}; root.performance={now:function(){return Date.now();}, timing:{navigationStart:Date.now()}}; root.Date.prototype.getTimezoneOffset=function(){return -480;};
 root.__bdmsCalls=[]; function FakeXHR(){ this.headers={}; }
 FakeXHR.prototype.open=function(method,url,async){ this._opened={method:String(method),url:String(url),async:async!==false}; root.__bdmsCalls.push({kind:'open',method:String(method),url:String(url),async:async!==false}); };
 FakeXHR.prototype.send=function(body){ root.__bdmsCalls.push({kind:'send',body:body==null?'':String(body).slice(0,200),opened:this._opened}); };
 FakeXHR.prototype.setRequestHeader=function(k,v){ this.headers[String(k)]=String(v); root.__bdmsCalls.push({kind:'setHeader',k:String(k),v:String(v)}); };
 FakeXHR.prototype.addEventListener=function(){}; FakeXHR.prototype.removeEventListener=function(){}; FakeXHR.prototype.overrideMimeType=function(){};
 root.XMLHttpRequest=FakeXHR; root.fetch=function(input, init){ root.__bdmsCalls.push({kind:'fetch',input:String(input),init:init}); return makeThenable({ok:true,status:200,text:function(){return makeThenable('');},json:function(){return makeThenable({});}}); };
})();
`
	_, err := vm.RunString(env)
	return err
}

func ensureBDMSMsTokenInURL(rawURL string, cookie string, externalMsToken string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if q.Get("msToken") == "" {
		if externalMsToken == "" {
			externalMsToken = pickCookieValueForBDMS(cookie, "msToken")
		}
		if externalMsToken == "" {
			externalMsToken = utils.GenerateMsToken(172)
		}
		q.Set("msToken", externalMsToken)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func urlHasQueryKey(rawURL string, key string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return strings.Contains(rawURL, key+"=")
	}
	_, ok := u.Query()[key]
	return ok
}

func pickCookieValueForBDMS(cookie string, name string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		idx := strings.IndexByte(part, '=')
		if idx <= 0 {
			continue
		}
		if strings.TrimSpace(part[:idx]) != name {
			continue
		}
		value, err := url.QueryUnescape(strings.TrimSpace(part[idx+1:]))
		if err != nil {
			return strings.TrimSpace(part[idx+1:])
		}
		return value
	}
	return ""
}

func firstNonEmptyBDMSString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func queryParamLengths(rawURL string, keys ...string) map[string]int {
	lengths := map[string]int{}
	u, err := url.Parse(rawURL)
	if err != nil {
		return lengths
	}
	q := u.Query()
	for _, key := range keys {
		if value := q.Get(key); value != "" {
			lengths[key] = len(value)
		}
	}
	return lengths
}

func queryValueLength(rawURL string, key string) int {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	return len(u.Query().Get(key))
}

func redactSignedURLForLog(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		replacer := rawURL
		for _, key := range []string{"msToken", "a_bogus", "X-Bogus", "_signature"} {
			replacer = redactQueryValue(replacer, key)
		}
		return replacer
	}
	q := u.Query()
	for _, key := range []string{"msToken", "a_bogus", "X-Bogus", "_signature"} {
		if values, ok := q[key]; ok && len(values) > 0 {
			q.Set(key, fmt.Sprintf("<redacted:%d>", len(values[0])))
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func redactQueryValue(rawURL string, key string) string {
	marker := key + "="
	idx := strings.Index(rawURL, marker)
	if idx < 0 {
		return rawURL
	}
	start := idx + len(marker)
	end := strings.IndexByte(rawURL[start:], '&')
	if end < 0 {
		end = len(rawURL)
	} else {
		end += start
	}
	return rawURL[:start] + "<redacted>" + rawURL[end:]
}
