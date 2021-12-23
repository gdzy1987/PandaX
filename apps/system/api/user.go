package api

import (
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/kakuilan/kgo"
	"github.com/mssola/user_agent"
	"net/http"
	"os"
	form2 "pandax/apps/system/api/form"
	vo2 "pandax/apps/system/api/vo"
	entity2 "pandax/apps/system/entity"
	services2 "pandax/apps/system/services"
	"pandax/base/biz"
	"pandax/base/captcha"
	"pandax/base/config"
	"pandax/base/ctx"
	"pandax/base/ginx"
	"pandax/base/global"
	"pandax/base/utils"
	"strings"
	"time"
)

type UserApi struct {
	UserApp     services2.SysUserModel
	MenuApp     services2.SysMenuModel
	PostApp     services2.SysPostModel
	RoleApp     services2.SysRoleModel
	RoleMenuApp services2.SysRoleMenuModel
	DeptApp     services2.SysDeptModel
	LogLogin    services2.LogLoginModel
}

// @Tags Base
// @Summary 获取验证码
// @Produce  application/json
// @Success 200 {string} string "{"success":true,"data":{},"msg":"登陆成功"}"
// @Router /system/user/getCaptcha [get]
func (u *UserApi) GenerateCaptcha(c *gin.Context) {
	id, image := captcha.Generate()
	c.JSON(http.StatusOK, map[string]interface{}{"base64Captcha": image, "captchaId": id})
}

// @Tags Base
// @Summary 刷新token
// @Produce  application/json
// @Success 200 {string} string "{"success":true,"data":{},"msg":"登陆成功"}"
// @Router /system/user/refreshToken [get]
func (u *UserApi) RefreshToken(rc *ctx.ReqCtx) {
	tokenStr := rc.GinCtx.Request.Header.Get("X-TOKEN")
	token, err := ctx.RefreshToken(tokenStr)
	biz.ErrIsNil(err, "刷新token失败")
	rc.ResData = map[string]interface{}{"token": token, "expire": time.Now().Unix() + config.Conf.Jwt.ExpireTime}
}

// @Tags Base
// @Summary 用户登录
// @Produce  application/json
// @Param data body form.Login true "用户名, 密码, 验证码"
// @Success 200 {string} string "{"success":true,"data":{},"msg":"登陆成功"}"
// @Router /system/user/login [post]
func (u *UserApi) Login(rc *ctx.ReqCtx) {
	var l form2.Login
	ginx.BindJsonAndValid(rc.GinCtx, &l)
	biz.IsTrue(captcha.Verify(l.CaptchaId, l.Captcha), "验证码认证失败")

	login := u.UserApp.Login(entity2.Login{Username: l.Username, Password: l.Password})
	role := u.RoleApp.FindOne(login.RoleId)

	token, err := ctx.CreateToken(
		ctx.Claims{
			UserId:   login.UserId,
			UserName: login.Username,
			RoleId:   login.RoleId,
			RoleKey:  role.RoleKey,
			DeptId:   login.DeptId,
			PostId:   login.PostId,
			StandardClaims: jwt.StandardClaims{
				NotBefore: time.Now().Unix() - 1000,                       // 签名生效时间
				ExpiresAt: time.Now().Unix() + config.Conf.Jwt.ExpireTime, // 过期时间 7天  配置文件
				Issuer:    "PandaX",                                       // 签名的发行者
			},
		})

	biz.ErrIsNil(err, "生成Token失败")
	//前端权限
	permis := u.RoleMenuApp.GetPermis(role.RoleId)
	menus := u.MenuApp.SelectMenuRole(role.RoleKey)

	rc.ResData = map[string]interface{}{
		"user":        login,
		"permissions": permis,
		"menus":       Build(*menus),
		"token":       token,
		"expire":      time.Now().Unix() + config.Conf.Jwt.ExpireTime,
	}

	var loginLog entity2.LogLogin
	ua := user_agent.New(rc.GinCtx.Request.UserAgent())
	loginLog.Ipaddr = rc.GinCtx.ClientIP()
	loginLog.LoginLocation = utils.GetRealAddressByIP(rc.GinCtx.ClientIP())
	loginLog.LoginTime = time.Now()
	loginLog.Status = "0"
	loginLog.Remark = rc.GinCtx.Request.UserAgent()
	browserName, browserVersion := ua.Browser()
	loginLog.Browser = browserName + " " + browserVersion
	loginLog.Os = ua.OS()
	loginLog.Platform = ua.Platform()
	loginLog.Username = login.Username
	loginLog.Msg = "登录成功"
	loginLog.CreateBy = login.Username
	u.LogLogin.Insert(loginLog)
}

// @Tags Base
// @Summary 退出登录
// @Produce  application/json
// @Success 200 {string} string "{"success":true,"data":{},"msg":"登陆成功"}"
// @Router /system/user/logout [post]
func (u *UserApi) LogOut(rc *ctx.ReqCtx) {
	var loginLog entity2.LogLogin
	ua := user_agent.New(rc.GinCtx.Request.UserAgent())
	loginLog.Ipaddr = rc.GinCtx.ClientIP()
	loginLog.LoginTime = time.Now()
	loginLog.Status = "0"
	loginLog.Remark = rc.GinCtx.Request.UserAgent()
	browserName, browserVersion := ua.Browser()
	loginLog.Browser = browserName + " " + browserVersion
	loginLog.Os = ua.OS()
	loginLog.Platform = ua.Platform()
	loginLog.Username = rc.LoginAccount.UserName
	loginLog.Msg = "退出成功"
	u.LogLogin.Insert(loginLog)
}

// @Summary 列表数据
// @Description 获取JSON
// @Tags 用户
// @Param userName query string false "userName"
// @Param phone query string false "phone"
// @Param status query string false "status"
// @Param pageSize query int false "页条数"
// @Param pageNum query int false "页码"
// @Success 200 {string} string "{"code": 200, "data": [...]}"
// @Success 200 {string} string "{"code": -1, "message": "抱歉未找到相关信息"}"
// @Router /system/user/sysUserList [get]
// @Security X-TOKEN
func (u *UserApi) GetSysUserList(rc *ctx.ReqCtx) {
	pageNum := ginx.QueryInt(rc.GinCtx, "pageNum", 1)
	pageSize := ginx.QueryInt(rc.GinCtx, "pageSize", 10)
	status := rc.GinCtx.Query("status")
	userName := rc.GinCtx.Query("username")
	phone := rc.GinCtx.Query("phone")
	deptId := ginx.QueryInt(rc.GinCtx, "deptId", 0)
	var user entity2.SysUser
	user.Status = status
	user.Username = userName
	user.Phone = phone
	user.DeptId = int64(deptId)
	list, total := u.UserApp.FindListPage(pageNum, pageSize, user)

	rc.ResData = map[string]interface{}{
		"data":     list,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}
}

// @Summary 获取当前登录用户
// @Description 获取JSON
// @Tags 个人中心
// @Success 200 {string} string "{"code": 200, "data": [...]}"
// @Router /system/user/profile [get]
// @Security
func (u *UserApi) GetSysUserProfile(rc *ctx.ReqCtx) {

	sysUser := entity2.SysUser{}
	sysUser.UserId = rc.LoginAccount.UserId
	user := u.UserApp.FindOne(sysUser)

	//获取角色列表
	roleList := u.RoleApp.FindList(entity2.SysRole{RoleId: rc.LoginAccount.RoleId})
	//岗位列表
	postList := u.PostApp.FindList(entity2.SysPost{PostId: rc.LoginAccount.PostId})
	//获取部门列表
	deptList := u.DeptApp.FindList(entity2.SysDept{DeptId: rc.LoginAccount.DeptId})

	postIds := make([]int64, 0)
	postIds = append(postIds, rc.LoginAccount.PostId)

	roleIds := make([]int64, 0)
	roleIds = append(roleIds, rc.LoginAccount.RoleId)

	rc.ResData = map[string]interface{}{
		"data":    user,
		"postIds": postIds,
		"roleIds": roleIds,
		"roles":   roleList,
		"posts":   postList,
		"dept":    deptList,
	}
}

// @Summary 修改头像
// @Description 修改头像
// @Tags 用户
// @Param file formData file true "file"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "添加失败"}"
// @Router /system/user/profileAvatar [post]
func (u *UserApi) InsetSysUserAvatar(rc *ctx.ReqCtx) {
	form, err := rc.GinCtx.MultipartForm()
	biz.ErrIsNil(err, "头像上传失败")

	files := form.File["upload[]"]
	guid, _ := kgo.KStr.UuidV4()
	filPath := "static/uploadfile/" + guid + ".jpg"
	for _, file := range files {
		global.Log.Info(file.Filename)
		// 上传文件至指定目录
		biz.ErrIsNil(rc.GinCtx.SaveUploadedFile(file, filPath), "保存头像失败")
	}
	sysuser := entity2.SysUser{}
	sysuser.UserId = rc.LoginAccount.UserId
	sysuser.Avatar = "/" + filPath
	sysuser.UpdateBy = rc.LoginAccount.UserName

	u.UserApp.Update(sysuser)
}

// @Summary 修改密码
// @Description 修改密码
// @Tags 用户
// @Param pwd body entity.SysUserPwd true "pwd"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "添加失败"}"
// @Router /system/user/updatePwd [post]
func (u *UserApi) SysUserUpdatePwd(rc *ctx.ReqCtx) {
	var pws entity2.SysUserPwd
	ginx.BindJsonAndValid(rc.GinCtx, &pws)

	user := entity2.SysUser{}
	user.UserId = rc.LoginAccount.UserId
	u.UserApp.SetPwd(user, pws)
}

// @Summary 获取用户
// @Description 获取JSON
// @Tags 用户
// @Param userId path int true "用户编码"
// @Success 200 {object} app.Response "{"code": 200, "data": [...]}"
// @Router /system/user/sysUser/{userId} [get]
// @Security
func (u *UserApi) GetSysUser(rc *ctx.ReqCtx) {
	userId := ginx.PathParamInt(rc.GinCtx, "userId")

	user := entity2.SysUser{}
	user.UserId = int64(userId)
	result := u.UserApp.FindOne(user)

	roles := u.RoleApp.FindList(entity2.SysRole{})

	posts := u.PostApp.FindList(entity2.SysPost{})

	rc.ResData = map[string]interface{}{
		"data":    result,
		"postIds": result.PostIds,
		"roleIds": result.RoleIds,
		"roles":   roles,
		"posts":   posts,
	}
}

// @Summary 获取添加用户角色和职位
// @Description 获取JSON
// @Tags 用户
// @Success 200 {string} string "{"code": 200, "data": [...]}"
// @Router /system/user/getInit [get]
// @Security
func (u *UserApi) GetSysUserInit(rc *ctx.ReqCtx) {
	roles := u.RoleApp.FindList(entity2.SysRole{})

	posts := u.PostApp.FindList(entity2.SysPost{})
	mp := make(map[string]interface{}, 2)
	mp["roles"] = roles
	mp["posts"] = posts
	rc.ResData = mp
}

// @Summary 获取添加用户角色和职位
// @Description 获取JSON
// @Tags 用户
// @Success 200 {string} string "{"code": 200, "data": [...]}"
// @Router /system/user/getInit [get]
// @Security
func (u *UserApi) GetUserRolePost(rc *ctx.ReqCtx) {
	var user entity2.SysUser
	user.UserId = rc.LoginAccount.UserId

	resData := u.UserApp.FindOne(user)

	roles := make([]entity2.SysRole, 0)
	posts := make([]entity2.SysPost, 0)
	for _, roleId := range strings.Split(resData.RoleIds, ",") {
		ro := u.RoleApp.FindOne(kgo.KConv.Str2Int64(roleId))
		roles = append(roles, *ro)
	}
	for _, postId := range strings.Split(resData.PostIds, ",") {
		po := u.PostApp.FindOne(kgo.KConv.Str2Int64(postId))
		posts = append(posts, *po)
	}
	mp := make(map[string]interface{}, 2)
	mp["roles"] = roles
	mp["posts"] = posts
	rc.ResData = mp
}

// @Summary 创建用户
// @Description 获取JSON
// @Tags 用户
// @Accept  application/json
// @Product application/json
// @Param data body entity.SysUser true "用户数据"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": 400, "message": "添加失败"}"
// @Router /system/user/sysUser [post]
func (u *UserApi) InsertSysUser(rc *ctx.ReqCtx) {
	var sysUser entity2.SysUser
	ginx.BindJsonAndValid(rc.GinCtx, &sysUser)
	sysUser.CreateBy = rc.LoginAccount.UserName
	u.UserApp.Insert(sysUser)
}

// @Summary 修改用户数据
// @Description 获取JSON
// @Tags 用户
// @Accept  application/json
// @Product application/json
// @Param data body entity.SysUser true "用户数据"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": 400, "message": "添加失败"}"
// @Router /system/user/sysUser [put]
func (u *UserApi) UpdateSysUser(rc *ctx.ReqCtx) {
	var sysUser entity2.SysUser
	ginx.BindJsonAndValid(rc.GinCtx, &sysUser)
	sysUser.CreateBy = rc.LoginAccount.UserName
	u.UserApp.Update(sysUser)
}

// @Summary 修改用户状态
// @Description 获取JSON
// @Tags 用户
// @Accept  application/json
// @Product application/json
// @Param data body entity.SysUser true "用户数据"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": 400, "message": "添加失败"}"
// @Router /system/user/sysUser [put]
func (u *UserApi) UpdateSysUserStu(rc *ctx.ReqCtx) {
	var sysUser entity2.SysUser
	ginx.BindJsonAndValid(rc.GinCtx, &sysUser)
	sysUser.CreateBy = rc.LoginAccount.UserName
	u.UserApp.Update(sysUser)
}

// @Summary 删除用户数据
// @Description 删除数据
// @Tags 用户
// @Param userId path int true "多个id 使用逗号隔开"
// @Success 200 {string} string	"{"code": 200, "message": "删除成功"}"
// @Success 200 {string} string	"{"code": 400, "message": "删除失败"}"
// @Router /system/user/sysuser/{userId} [delete]
func (u *UserApi) DeleteSysUser(rc *ctx.ReqCtx) {
	userIds := rc.GinCtx.Param("userId")
	us := utils.IdsStrToIdsIntGroup(userIds)
	u.UserApp.Delete(us)
}

// @Summary 导出用户
// @Description 导出数据
// @Tags 用户
// @Param userName query string false "userName"
// @Param phone query string false "phone"
// @Param status query string false "status"
// @Success 200 {string} string	"{"code": 200, "message": "删除成功"}"
// @Success 200 {string} string	"{"code": 400, "message": "删除失败"}"
// @Router /system/dict/type/export [get]
func (u *UserApi) ExportUser(rc *ctx.ReqCtx) {
	status := rc.GinCtx.Query("status")
	userName := rc.GinCtx.Query("username")
	phone := rc.GinCtx.Query("phone")

	var user entity2.SysUser
	user.Status = status
	user.Username = userName
	user.Phone = phone
	list := u.UserApp.FindList(user)
	fileName := utils.GetFileName(config.Conf.Server.ExcelDir, "用户")
	utils.InterfaceToExcel(*list, fileName)

	line, err := kgo.KFile.ReadFile(fileName)
	if err != nil {
		os.Remove(fileName)
		biz.ErrIsNil(err, "读取文件失败")
	}
	rc.Download(line, fileName)
}

// 构建前端路由
func Build(menus []entity2.SysMenu) []vo2.RouterVo {
	equals := func(a string, b string) bool {
		if a == b {
			return true
		}
		return false
	}
	if len(menus) == 0 {

	}
	rvs := make([]vo2.RouterVo, 0)
	for _, ms := range menus {
		var rv vo2.RouterVo
		rv.Name = ms.Path
		rv.Path = ms.Path
		rv.Component = ms.Component
		auth := make([]string, 0)
		if ms.Permission != "" {
			auth = strings.Split(ms.Permission, ",")
		}
		rv.Meta = vo2.MetaVo{
			Title:       ms.MenuName,
			IsLink:      ms.IsLink,
			IsHide:      equals("1", ms.IsHide),
			IsKeepAlive: equals("0", ms.IsKeepAlive),
			IsAffix:     equals("0", ms.IsAffix),
			IsFrame:     equals("0", ms.IsFrame),
			Auth:        auth,
			Icon:        ms.Icon,
		}
		rv.Children = Build(ms.Children)
		rvs = append(rvs, rv)
	}

	return rvs
}