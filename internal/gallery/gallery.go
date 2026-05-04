package gallery

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var w1251 = [128]rune{
	0x0402, 0x0403, 0x201A, 0x0453, 0x201E, 0x2026, 0x2020, 0x2021,
	0x20AC, 0x2030, 0x0409, 0x2039, 0x040A, 0x040C, 0x040B, 0x040F,
	0x0452, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0xFFFD, 0x2122, 0x0459, 0x203A, 0x045A, 0x045C, 0x045B, 0x045F,
	0x00A0, 0x040E, 0x045E, 0x0408, 0x00A4, 0x0490, 0x00A6, 0x00A7,
	0x0401, 0x00A9, 0x0404, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x0407,
	0x00B0, 0x00B1, 0x0406, 0x0456, 0x0491, 0x00B5, 0x00B6, 0x00B7,
	0x0451, 0x2116, 0x0454, 0x00BB, 0x0458, 0x0405, 0x0455, 0x0457,
	0x0410, 0x0411, 0x0412, 0x0413, 0x0414, 0x0415, 0x0416, 0x0417,
	0x0418, 0x0419, 0x041A, 0x041B, 0x041C, 0x041D, 0x041E, 0x041F,
	0x0420, 0x0421, 0x0422, 0x0423, 0x0424, 0x0425, 0x0426, 0x0427,
	0x0428, 0x0429, 0x042A, 0x042B, 0x042C, 0x042D, 0x042E, 0x042F,
	0x0430, 0x0431, 0x0432, 0x0433, 0x0434, 0x0435, 0x0436, 0x0437,
	0x0438, 0x0439, 0x043A, 0x043B, 0x043C, 0x043D, 0x043E, 0x043F,
	0x0440, 0x0441, 0x0442, 0x0443, 0x0444, 0x0445, 0x0446, 0x0447,
	0x0448, 0x0449, 0x044A, 0x044B, 0x044C, 0x044D, 0x044E, 0x044F,
}

func decodeW1251(b []byte) string {
	var buf [4]byte
	sb := strings.Builder{}
	sb.Grow(len(b))
	for _, c := range b {
		if c < 0x80 {
			sb.WriteByte(c)
		} else {
			n := utf8.EncodeRune(buf[:], w1251[c-0x80])
			sb.Write(buf[:n])
		}
	}
	return sb.String()
}

var peerRE = regexp.MustCompile(`<a href="([^"/]+)/messages0\.html">([^<]+)</a>`)

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

type chatEntry struct {
	id         string
	name       string
	photoCount int
}

func countImages(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			n++
		}
	}
	return n
}

func listImages(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var imgs []string
	for _, e := range entries {
		if !e.IsDir() && imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			imgs = append(imgs, e.Name())
		}
	}
	sort.Strings(imgs)
	return imgs
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}

func writeIndex(path string, chats []chatEntry) error {
	sb := strings.Builder{}
	sb.WriteString(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>VK — Media</title>
  <link rel="shortcut icon" href="../favicon.ico">
  <link rel="stylesheet" type="text/css" href="../style.css">
  <style>
    .sort-bar{padding:8px 0 12px;display:flex;align-items:center;gap:10px;font-size:13px}
    .sort-bar select{font-size:13px;padding:4px 8px;border:1px solid #ccc;border-radius:4px;cursor:pointer}
  </style>
</head>
<body>
  <div class="wrap">
    <div class="header">
  <div class="page_header">
    <div class="top_home_logo"></div>
  </div>
</div>
    <div class="page_content clear_fix">
<div class="page_block">
<!--content-->
 <h2 class="page_block_h2">
<div class="page_block_header clear_fix">
  <div class="page_block_header_extra_left _header_extra_left"></div>
  <div class="page_block_header_extra _header_extra"></div>
  <div class="page_block_header_inner _header_inner" data-testid="header"><a class="ui_crumb" href="../index.html" data-testid="header_link_crumb">Profile</a><div class="ui_crumb_sep"></div><a class="ui_crumb" href="index-messages.html" data-testid="header_link_crumb">Messages</a><div class="ui_crumb_sep"></div><div class="ui_crumb">Media</div></div>
</div>
</h2><div class="wrap_page_content">
<div class="sort-bar">
  <span>Сортировка:</span>
  <select id="sort-select" onchange="sortChats(this.value)">
    <option value="default">по умолчанию</option>
    <option value="desc">наибольшее</option>
    <option value="asc">наименьшее</option>
  </select>
</div>
<div id="chat-list">
`)
	for _, c := range chats {
		fmt.Fprintf(&sb,
			"  <div class=\"item\" data-count=\"%d\" data-name=\"%s\">\n  <div class='item__main'><div class=\"message-peer\">\n  <div class=\"message-peer--id\">\n    <a href=\"%s/media-gallery.html\">%s &mdash; ID: %s &mdash; %d фото</a>\n  </div>\n</div></div>\n  \n  \n</div>\n",
			c.photoCount, htmlEscape(c.name), htmlEscape(c.id), htmlEscape(c.name), htmlEscape(c.id), c.photoCount)
	}
	sb.WriteString(`</div>
<script>
(function(){
  var list = document.getElementById('chat-list');
  var original = Array.from(list.children);
  window.sortChats = function(order) {
    var items = Array.from(list.children);
    if (order === 'default') {
      original.forEach(function(el){ list.appendChild(el); });
    } else {
      items.sort(function(a, b){
        var ca = parseInt(a.dataset.count, 10);
        var cb = parseInt(b.dataset.count, 10);
        return order === 'desc' ? cb - ca : ca - cb;
      });
      items.forEach(function(el){ list.appendChild(el); });
    }
  };
})();
</script>
</div>
<!--/content-->
</div>
   </div>
   <div class="footer">

   </div>
  </div>
</body>
</html>
`)
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

const galleryStyle = `<style>
.gallery-grid{display:grid;grid-template-columns:repeat(3,1fr);gap:10px;padding:12px 0}
.gallery-grid a{display:block;overflow:hidden;aspect-ratio:1;background:#000}
.gallery-grid img{width:100%;height:100%;object-fit:cover;transition:opacity .15s}
.gallery-grid img:hover{opacity:.85;cursor:pointer}
#lb{display:none;position:fixed;inset:0;background:rgba(0,0,0,.92);z-index:9999;align-items:center;justify-content:center}
#lb.open{display:flex}
#lb-img{max-width:90vw;max-height:90vh;object-fit:contain;border-radius:2px}
#lb-counter{position:absolute;top:14px;right:20px;color:#fff;font-size:14px;font-family:sans-serif;opacity:.8}
#lb-close{position:absolute;top:10px;left:16px;color:#fff;font-size:26px;cursor:pointer;opacity:.7;line-height:1;font-family:sans-serif}
#lb-close:hover{opacity:1}
.lb-arrow{position:absolute;top:50%;transform:translateY(-50%);background:rgba(255,255,255,.12);border:none;color:#fff;font-size:36px;padding:10px 18px;cursor:pointer;border-radius:4px;line-height:1;font-family:sans-serif}
.lb-arrow:hover{background:rgba(255,255,255,.25)}
#lb-prev{left:12px}
#lb-next{right:12px}
</style>`

const galleryScript = `<script>
(function(){
  var imgs = window._galleryImgs;
  var cur = 0;
  var lb = document.getElementById('lb');
  var lbImg = document.getElementById('lb-img');
  var lbCounter = document.getElementById('lb-counter');

  function show(i){
    cur = (i + imgs.length) % imgs.length;
    lbImg.src = imgs[cur];
    lbCounter.textContent = (cur+1) + ' / ' + imgs.length;
  }
  function open(i){ show(i); lb.classList.add('open'); }
  function close(){ lb.classList.remove('open'); lbImg.src=''; }

  document.querySelectorAll('.gallery-grid img').forEach(function(img, i){
    img.parentNode.addEventListener('click', function(e){ e.preventDefault(); open(i); });
  });
  document.getElementById('lb-close').addEventListener('click', close);
  document.getElementById('lb-prev').addEventListener('click', function(){ show(cur-1); });
  document.getElementById('lb-next').addEventListener('click', function(){ show(cur+1); });
  lb.addEventListener('click', function(e){ if(e.target===lb) close(); });
  document.addEventListener('keydown', function(e){
    if(!lb.classList.contains('open')) return;
    if(e.key==='ArrowLeft') show(cur-1);
    else if(e.key==='ArrowRight') show(cur+1);
    else if(e.key==='Escape') close();
  });
})();
</script>`

func writeGallery(path, chatID, chatName string, imgs []string) error {
	sb := strings.Builder{}
	fmt.Fprintf(&sb, `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>%s — Media</title>
  <link rel="shortcut icon" href="../../favicon.ico">
  <link rel="stylesheet" type="text/css" href="../../style.css">
%s
</head>
<body>
  <div class="wrap">
    <div class="header">
  <div class="page_header">
    <div class="top_home_logo"></div>
  </div>
</div>
    <div class="page_content page_block">
      <h2 class="page_block_h2">
<div class="page_block_header clear_fix">
  <div class="page_block_header_extra_left _header_extra_left"></div>
  <div class="page_block_header_extra _header_extra"></div>
  <div class="page_block_header_inner _header_inner" data-testid="header"><a class="ui_crumb" href="../../index.html" data-testid="header_link_crumb">Profile</a><div class="ui_crumb_sep"></div><a class="ui_crumb" href="../index-messages-with-media.html" data-testid="header_link_crumb">Media</a><div class="ui_crumb_sep"></div><div class="ui_crumb">%s</div></div>
</div>
</h2>
      <div class="wrap_page_content">
`, htmlEscape(chatName), galleryStyle, htmlEscape(chatName))

	sb.WriteString("<div class=\"gallery-grid\">\n")
	for _, img := range imgs {
		escaped := htmlEscape(img)
		fmt.Fprintf(&sb, "  <a href=\"media/%s\"><img src=\"media/%s\" loading=\"lazy\" alt=\"\"></a>\n", escaped, escaped)
	}
	sb.WriteString("</div>\n")

	sb.WriteString(`<div id="lb">
  <span id="lb-close">&times;</span>
  <span id="lb-counter"></span>
  <button class="lb-arrow" id="lb-prev">&#8592;</button>
  <img id="lb-img" src="" alt="">
  <button class="lb-arrow" id="lb-next">&#8594;</button>
</div>
`)

	sb.WriteString("<script>window._galleryImgs=[")
	for i, img := range imgs {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%q", "media/"+img)
	}
	sb.WriteString("];</script>\n")
	sb.WriteString(galleryScript)

	sb.WriteString(`      </div>
    </div>
  </div>
</body>
</html>
`)
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

const indexLinkSnippet = `<div class="item">
  <div class='item__main'><div class="index-row">
  <a class="menu__page-link" href="messages/index-messages-with-media.html">
    <span class="archive__page-icon archive__page-icon--index-messages"></span>
    <div>Media from messages</div>
  </a>
</div></div>


</div>`

func PatchIndexHTML(indexPath string) error {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	s := string(data)
	marker := `href="messages/index-messages.html"`
	if !strings.Contains(s, marker) {
		return fmt.Errorf("marker not found in index.html")
	}
	if strings.Contains(s, "index-messages-with-media.html") {
		return nil
	}
	idx := strings.Index(s, marker)
	closeBlock := "</div></div>\n  \n  \n</div>"
	rel := strings.Index(s[idx:], closeBlock)
	if rel < 0 {
		return fmt.Errorf("could not find closing block after Messages link")
	}
	insertAt := idx + rel + len(closeBlock)
	patched := s[:insertAt] + indexLinkSnippet + s[insertAt:]
	return os.WriteFile(indexPath, []byte(patched), 0644)
}

// Generate rebuilds index-messages-with-media.html and all per-chat media-gallery.html pages.
func Generate(archivePath string) error {
	messagesDir := filepath.Join(archivePath, "messages")

	indexData, err := os.ReadFile(filepath.Join(messagesDir, "index-messages.html"))
	if err != nil {
		return fmt.Errorf("read index-messages.html: %w", err)
	}
	decoded := decodeW1251(indexData)
	matches := peerRE.FindAllStringSubmatch(decoded, -1)
	nameByID := make(map[string]string, len(matches))
	for _, m := range matches {
		nameByID[m[1]] = m[2]
	}

	entries, err := os.ReadDir(messagesDir)
	if err != nil {
		return fmt.Errorf("readdir messages: %w", err)
	}

	var chats []chatEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mediaDir := filepath.Join(messagesDir, e.Name(), "media")
		if _, err := os.Stat(mediaDir); os.IsNotExist(err) {
			continue
		}
		n := countImages(mediaDir)
		if n == 0 {
			continue
		}
		name := nameByID[e.Name()]
		if name == "" {
			name = e.Name()
		}
		chats = append(chats, chatEntry{id: e.Name(), name: name, photoCount: n})
	}
	sort.Slice(chats, func(i, j int) bool { return chats[i].name < chats[j].name })

	for _, c := range chats {
		mediaDir := filepath.Join(messagesDir, c.id, "media")
		imgs := listImages(mediaDir)
		outPath := filepath.Join(messagesDir, c.id, "media-gallery.html")
		if err := writeGallery(outPath, c.id, c.name, imgs); err != nil {
			log.Printf("gallery %s: %v", c.id, err)
		}
	}

	if err := writeIndex(filepath.Join(messagesDir, "index-messages-with-media.html"), chats); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	if err := PatchIndexHTML(filepath.Join(archivePath, "index.html")); err != nil {
		log.Printf("patch index.html: %v", err)
	}

	log.Printf("gallery: regenerated %d chat pages", len(chats))
	return nil
}
