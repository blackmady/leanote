package controllers

import (
	"github.com/revel/revel"
	//	"encoding/json"
	"github.com/leanote/leanote/app/info"
	. "github.com/leanote/leanote/app/lea"
	"gopkg.in/mgo.v2/bson"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"fmt"
	//	"github.com/leanote/leanote/app/types"
	//	"io/ioutil"
	//	"bytes"
	//	"os"
)

type Note struct {
	BaseController
}

// 笔记首页, 判断是否已登录
// 已登录, 得到用户基本信息(notebook, shareNotebook), 跳转到index.html中
// 否则, 转向登录页面
func (c Note) Index(noteId, online string) revel.Result {
	c.SetLocale()
	userInfo := c.GetUserAndBlogUrl()

	userId := userInfo.UserId.Hex()
	
	// 没有登录
	if userId == "" {
		return c.Redirect("/login")
	}
	
	c.RenderArgs["openRegister"] = configService.IsOpenRegister()

	// 已登录了, 那么得到所有信息
	notebooks := notebookService.GetNotebooks(userId)
	shareNotebooks, sharedUserInfos := shareService.GetShareNotebooks(userId)
	
	// 还需要按时间排序(DESC)得到notes
	notes := []info.Note{}
	noteContent := info.NoteContent{}
	
	if len(notebooks) > 0 {
		// noteId是否存在
		// 是否传入了正确的noteId
		hasRightNoteId := false
		if IsObjectId(noteId) {
			note := noteService.GetNoteById(noteId)
			
			if note.NoteId != "" {
				var noteOwner = note.UserId.Hex()
				noteContent = noteService.GetNoteContent(noteId, noteOwner)
				
				hasRightNoteId = true
				c.RenderArgs["curNoteId"] = noteId
				c.RenderArgs["curNotebookId"] = note.NotebookId.Hex()
				
				// 打开的是共享的笔记, 那么判断是否是共享给我的默认笔记
				if noteOwner != c.GetUserId() {
					if shareService.HasReadPerm(noteOwner, c.GetUserId(), noteId) {
						// 不要获取notebook下的笔记
						// 在前端下发请求
						c.RenderArgs["curSharedNoteNotebookId"] = note.NotebookId.Hex()
						c.RenderArgs["curSharedUserId"] = noteOwner;
					// 没有读写权限
					} else {
						hasRightNoteId = false
					}
				} else {
					_, notes = noteService.ListNotes(c.GetUserId(), note.NotebookId.Hex(), false, c.GetPage(), 50, defaultSortField, false, false);
					
					// 如果指定了某笔记, 则该笔记放在首位
					lenNotes := len(notes)
					if lenNotes > 1 {
						notes2 := make([]info.Note, len(notes))
						notes2[0] = note
						i := 1
						for _, note := range notes {
							if note.NoteId.Hex() != noteId {
								if i == lenNotes { // 防止越界
									break;
								}
								notes2[i] = note
								i++
							}
						}
						notes = notes2
					}
				}
			}
			
			// 得到最近的笔记
			_, latestNotes := noteService.ListNotes(c.GetUserId(), "", false, c.GetPage(), 50, defaultSortField, false, false);
			c.RenderArgs["latestNotes"] = latestNotes
		}
		
		// 没有传入笔记
		// 那么得到最新笔记
		if !hasRightNoteId {
			_, notes = noteService.ListNotes(c.GetUserId(), "", false, c.GetPage(), 50, defaultSortField, false, false);
			if len(notes) > 0 {
				noteContent = noteService.GetNoteContent(notes[0].NoteId.Hex(), userId)
				c.RenderArgs["curNoteId"] = notes[0].NoteId.Hex()
			}
		}
	}
	
	// 当然, 还需要得到第一个notes的content
	//...
	c.RenderArgs["isAdmin"] = configService.GetAdminUsername() == userInfo.Username
	
	c.RenderArgs["userInfo"] = userInfo
	c.RenderArgs["notebooks"] = notebooks
	c.RenderArgs["shareNotebooks"] = shareNotebooks // note信息在notes列表中
	c.RenderArgs["sharedUserInfos"] = sharedUserInfos
	
	c.RenderArgs["notes"] = notes
	c.RenderArgs["noteContentJson"] = noteContent
	c.RenderArgs["noteContent"] = noteContent.Content
	
	c.RenderArgs["tags"] = tagService.GetTags(c.GetUserId())
	
	c.RenderArgs["globalConfigs"] = configService.GetGlobalConfigForUser()
	
	// return c.RenderTemplate("note/note.html")

	if isDev, _ := revel.Config.Bool("mode.dev"); isDev && online == "" {
		return c.RenderTemplate("note/note-dev.html")
	} else {
		return c.RenderTemplate("note/note.html")
	}
}

// 首页, 判断是否已登录
// 已登录, 得到用户基本信息(notebook, shareNotebook), 跳转到index.html中
// 否则, 转向登录页面
func (c Note) ListNotes(notebookId string) revel.Result {
	_, notes := noteService.ListNotes(c.GetUserId(), notebookId, false, c.GetPage(), pageSize, defaultSortField, false, false)
	return c.RenderJson(notes)
}

// 得到trash
func (c Note) ListTrashNotes() revel.Result {
	_, notes := noteService.ListNotes(c.GetUserId(), "", true, c.GetPage(), pageSize, defaultSortField, false, false)
	return c.RenderJson(notes)
}

// 得到note和内容
func (c Note) GetNoteAndContent(noteId string) revel.Result {
	return c.RenderJson(noteService.GetNoteAndContent(noteId, c.GetUserId()))
}

// 得到内容
func (c Note) GetNoteContent(noteId string) revel.Result {
	noteContent := noteService.GetNoteContent(noteId, c.GetUserId())
	return c.RenderJson(noteContent)
}

// 更新note或content
// 肯定会传userId(谁的), NoteId
// 会传Title, Content, Tags, 一种或几种
type NoteOrContent struct {
	NotebookId string
	NoteId string
	UserId string
	Title string
	Desc string
	ImgSrc string
	Tags string
	Content string
	Abstract string
	IsNew bool
	IsMarkdown bool
	FromUserId string // 为共享而新建
	IsBlog bool // 是否是blog, 更新note不需要修改, 添加note时才有可能用到, 此时需要判断notebook是否设为Blog
}
// 这里不能用json, 要用post
func (c Note) UpdateNoteOrContent(noteOrContent NoteOrContent) revel.Result {
	// 新添加note
	if noteOrContent.IsNew {
		userId := c.GetObjectUserId();
//		myUserId := userId
		// 为共享新建?
		if noteOrContent.FromUserId != "" {
			userId = bson.ObjectIdHex(noteOrContent.FromUserId)
		}
		
		note := info.Note{UserId: userId, 
			NoteId: bson.ObjectIdHex(noteOrContent.NoteId), 
			NotebookId: bson.ObjectIdHex(noteOrContent.NotebookId), 
			Title: noteOrContent.Title, 
			Tags: strings.Split(noteOrContent.Tags, ","),
			Desc: noteOrContent.Desc,
			ImgSrc: noteOrContent.ImgSrc,
			IsBlog: noteOrContent.IsBlog,
			IsMarkdown: noteOrContent.IsMarkdown,
		};
		noteContent := info.NoteContent{NoteId: note.NoteId, 
			UserId: userId, 
			IsBlog: note.IsBlog,
			Content: noteOrContent.Content, 
			Abstract: noteOrContent.Abstract};
		
		note = noteService.AddNoteAndContentForController(note, noteContent, c.GetUserId())
		return c.RenderJson(note)
	}
	
	noteUpdate := bson.M{}
	needUpdateNote := false
	
	// Desc前台传来
	if c.Has("Desc") {
		needUpdateNote = true
		noteUpdate["Desc"] = noteOrContent.Desc;
	}
	if c.Has("ImgSrc") {
		needUpdateNote = true
		noteUpdate["ImgSrc"] = noteOrContent.ImgSrc;
	}
	if c.Has("Title") {
		needUpdateNote = true
		noteUpdate["Title"] = noteOrContent.Title;
	}
	
	if c.Has("Tags") {
		needUpdateNote = true
		noteUpdate["Tags"] = strings.Split(noteOrContent.Tags, ",");
	}
	
	// web端不控制
	if needUpdateNote { 
		noteService.UpdateNote(c.GetUserId(), 
			noteOrContent.NoteId, noteUpdate, -1)
	}
	
	//-------------
	afterContentUsn := 0
	contentOk := false
	contentMsg := ""
	if c.Has("Content") {
//		noteService.UpdateNoteContent(noteOrContent.UserId, c.GetUserId(), 
//			noteOrContent.NoteId, noteOrContent.Content, noteOrContent.Abstract)
		contentOk, contentMsg, afterContentUsn = noteService.UpdateNoteContent(c.GetUserId(),
			noteOrContent.NoteId, noteOrContent.Content, noteOrContent.Abstract, needUpdateNote, -1)
	}
	
	Log(afterContentUsn)
	Log(contentOk)
	Log(contentMsg)
	
	return c.RenderJson(true)
}

// 删除note/ 删除别人共享给我的笔记
// userId 是note.UserId
func (c Note) DeleteNote(noteId, userId string, isShared bool) revel.Result {
	if(!isShared) {
		return c.RenderJson(trashService.DeleteNote(noteId, c.GetUserId()));
	}
	
	return c.RenderJson(trashService.DeleteSharedNote(noteId, userId, c.GetUserId()));
}
// 删除trash
func (c Note) DeleteTrash(noteId string) revel.Result {
	return c.RenderJson(trashService.DeleteTrash(noteId, c.GetUserId()));
}
// 移动note
func (c Note) MoveNote(noteId, notebookId string) revel.Result {
	return c.RenderJson(noteService.MoveNote(noteId, notebookId, c.GetUserId()));
}
// 复制note
func (c Note) CopyNote(noteId, notebookId string) revel.Result {
	return c.RenderJson(noteService.CopyNote(noteId, notebookId, c.GetUserId()));
}
// 复制别人共享的笔记给我
func (c Note) CopySharedNote(noteId, notebookId, fromUserId string) revel.Result {
	return c.RenderJson(noteService.CopySharedNote(noteId, notebookId, fromUserId, c.GetUserId()));
}

//------------
// search
// 通过title搜索
func (c Note) SearchNote(key string) revel.Result {
	_, blogs := noteService.SearchNote(key, c.GetUserId(), c.GetPage(), pageSize, "UpdatedTime", false, false)
	return c.RenderJson(blogs)
}
// 通过tags搜索
func (c Note) SearchNoteByTags(tags []string) revel.Result {
	_, blogs := noteService.SearchNoteByTags(tags, c.GetUserId(), c.GetPage(), pageSize, "UpdatedTime", false)
	return c.RenderJson(blogs)
}

// 生成PDF
func (c Note) ToPdf(noteId, appKey string) revel.Result {
	// 虽然传了cookie但是这里还是不能得到userId, 所以还是通过appKey来验证之
	appKeyTrue, _ := revel.Config.String("app.secret")
	if appKeyTrue != appKey {
		return c.RenderText("error")
	}
	note := noteService.GetNoteById(noteId)
	if note.NoteId == "" {
		return c.RenderText("error")
	}

	noteUserId := note.UserId.Hex()
	content := noteService.GetNoteContent(noteId, noteUserId)
	userInfo := userService.GetUserInfo(noteUserId)

	//------------------
	// 将content的图片转换为base64
	contentStr := content.Content
	
	siteUrlPattern := configService.GetSiteUrl()
	if strings.Contains(siteUrlPattern, "https") {
		siteUrlPattern = strings.Replace(siteUrlPattern, "https", "https*", 1)
	} else {
		siteUrlPattern = strings.Replace(siteUrlPattern, "http", "https*", 1)
	}
	
	regImage, _ := regexp.Compile(`<img .*?(src=('|")` + siteUrlPattern + `/(file/outputImage|api/file/getImage)\?fileId=([a-z0-9A-Z]{24})("|'))`)

	findsImage := regImage.FindAllStringSubmatch(contentStr, -1) // 查找所有的
	//	[<img src="http://leanote.com/api/getImage?fileId=3354672e8d38f411286b000069" alt="" width="692" height="302" data-mce-src="http://leanote.com/file/outputImage?fileId=54672e8d38f411286b000069" src="http://leanote.com/file/outputImage?fileId=54672e8d38f411286b000069" " file/outputImage 54672e8d38f411286b000069 "]
	for _, eachFind := range findsImage {
		if len(eachFind) == 6 {
			fileId := eachFind[4]
			// 得到base64编码文件
			fileBase64 := fileService.GetImageBase64(noteUserId, fileId)
			if fileBase64 == "" {
				continue
			}

			// 1
			// src="http://leanote.com/file/outputImage?fileId=54672e8d38f411286b000069"
			allFixed := strings.Replace(eachFind[0], eachFind[1], "src=\""+fileBase64+"\"", -1)
			contentStr = strings.Replace(contentStr, eachFind[0], allFixed, -1)
		}
	}

	// markdown
	if note.IsMarkdown {
		// ![enter image description here](url)
		regImageMarkdown, _ := regexp.Compile(`!\[.*?\]\(` + siteUrlPattern + `/(file/outputImage|api/file/getImage)\?fileId=([a-z0-9A-Z]{24})\)`)
		findsImageMarkdown := regImageMarkdown.FindAllStringSubmatch(contentStr, -1) // 查找所有的
		for _, eachFind := range findsImageMarkdown {
			if len(eachFind) == 3 {
				fileId := eachFind[2]
				// 得到base64编码文件
				fileBase64 := fileService.GetImageBase64(noteUserId, fileId)
				if fileBase64 == "" {
					continue
				}

				// 1
				// src="http://leanote.com/file/outputImage?fileId=54672e8d38f411286b000069"
				allFixed := "![](" + fileBase64 + ")"
				contentStr = strings.Replace(contentStr, eachFind[0], allFixed, -1)
			}
		}
	}

	if note.Tags != nil && len(note.Tags) > 0 && note.Tags[0] != "" {
	} else {
		note.Tags = nil
	}
	c.RenderArgs["blog"] = note
	c.RenderArgs["content"] = contentStr
	c.RenderArgs["userInfo"] = userInfo
	userBlog := blogService.GetUserBlog(noteUserId)
	c.RenderArgs["userBlog"] = userBlog

	return c.RenderTemplate("file/pdf.html")
}

// 导出成PDF
func (c Note) ExportPdf(noteId string) revel.Result {
	re := info.NewRe()
	userId := c.GetUserId()
	note := noteService.GetNoteById(noteId)
	if note.NoteId == "" {
		re.Msg = "No Note"
		return c.RenderText("error")
	}

	noteUserId := note.UserId.Hex()
	// 是否有权限
	if noteUserId != userId {
		// 是否是有权限协作的
		if !note.IsBlog && !shareService.HasReadPerm(noteUserId, userId, noteId) {
			re.Msg = "No Perm"
			return c.RenderText("error")
		}
	}

	// path 判断是否需要重新生成之
	guid := NewGuid()
	fileUrlPath := "files/" + Digest3(noteUserId) + "/" + noteUserId + "/" + Digest2(guid) + "/images/pdf"
	dir := revel.BasePath + "/" + fileUrlPath
	if !MkdirAll(dir) {
		return c.RenderText("error, no dir")
	}
	filename := guid + ".pdf"
	path := dir + "/" + filename

	// leanote.com的secret
	appKey, _ := revel.Config.String("app.secretLeanote")
	if appKey == "" {
		appKey, _ = revel.Config.String("app.secret")
	}
	
	// 生成之
	binPath := configService.GetGlobalStringConfig("exportPdfBinPath")
	// 默认路径
	if binPath == "" {
		binPath = "/usr/local/bin/wkhtmltopdf"
	}

	url := configService.GetSiteUrl() + "/note/toPdf?noteId=" + noteId + "&appKey=" + appKey
	//	cc := binPath + " --no-stop-slow-scripts --javascript-delay 10000 \"" + url + "\"  \"" + path + "\"" //  \"" + cookieDomain + "\" \"" + cookieName + "\" \"" + cookieValue + "\""
	//	cc := binPath + " \"" + url + "\"  \"" + path + "\"" //  \"" + cookieDomain + "\" \"" + cookieName + "\" \"" + cookieValue + "\""
	// 等待--window-status为done的状态
	// http://madalgo.au.dk/~jakobt/wkhtmltoxdoc/wkhtmltopdf_0.10.0_rc2-doc.html
	// wkhtmltopdf参数大全
	var cc string
	if note.IsMarkdown {
		cc = binPath + " --window-status done \"" + url + "\"  \"" + path + "\"" //  \"" + cookieDomain + "\" \"" + cookieName + "\" \"" + cookieValue + "\""
	} else {
		cc = binPath + " \"" + url + "\"  \"" + path + "\"" //  \"" + cookieDomain + "\" \"" + cookieName + "\" \"" + cookieValue + "\""
	}

	cmd := exec.Command("/bin/sh", "-c", cc)
	_, err := cmd.Output()
	if err != nil {
		return c.RenderText("export pdf error. " + fmt.Sprintf("%v", err))
	}
	file, err := os.Open(path)
	if err != nil {
		return c.RenderText("export pdf error. " + fmt.Sprintf("%v", err))
	}
	// http://stackoverflow.com/questions/8588818/chrome-pdf-display-duplicate-headers-received-from-the-server
	//	filenameReturn = strings.Replace(filenameReturn, ",", "-", -1)
	filenameReturn := note.Title
	filenameReturn = FixFilename(filenameReturn)
	if filenameReturn == "" {
		filenameReturn = "Untitled.pdf"
	} else {
		filenameReturn += ".pdf"
	}
	return c.RenderBinary(file, filenameReturn, revel.Attachment, time.Now()) // revel.Attachment
}

// 设置/取消Blog; 置顶
func (c Note) SetNote2Blog(noteId string, isBlog, isTop bool) revel.Result {
	re := noteService.ToBlog(c.GetUserId(), noteId, isBlog, isTop)
	return c.RenderJson(re)
}
