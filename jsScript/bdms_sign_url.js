// Calls bdms.init and returns the URL rewritten by bdms.js.
(function(root){
 root.__signBDMSURL = function(candidateUrl){
  candidateUrl = String(candidateUrl || '');
  if (!root.bdms || typeof root.bdms.init !== 'function') {
   throw new Error('window.bdms.init is not available');
  }
  root.__bdmsCalls = [];
  root.bdms.init({
   aid: 6383,
   paths: ['/webcast/room/web/enter', '/webcast/im/fetch'],
   pageId: 1
  });
  var xhr = new root.XMLHttpRequest();
  xhr.open('GET', candidateUrl, true);
  xhr.send();
  for (var i = root.__bdmsCalls.length - 1; i >= 0; i--) {
   if (root.__bdmsCalls[i].kind === 'open') return String(root.__bdmsCalls[i].url || '');
  }
  return candidateUrl;
 };
})(globalThis);
