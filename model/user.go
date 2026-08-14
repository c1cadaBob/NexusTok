// Package model - user.go
// 该文件定义了用户数据模型和相关操作
//
// 用户是系统的核心实体，包含：
// - 基本信息：用户名、密码、邮箱、显示名
// - 认证信息：GitHub/Discord/OIDC/WeChat/Telegram ID、Access Token
// - 配额信息：总额度、已用额度、请求次数
// - 分组信息：用户所属分组（影响可用模型和倍率）
// - 邀请信息：邀请码、邀请人数、邀请奖励额度
// - 设置信息：用户个性化设置（JSON 格式）
// - 支付信息：Stripe 客户 ID
package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"                    // 公共工具包
	"github.com/c1cada/NexusTok/dto"                       // 数据传输对象
	"github.com/c1cada/NexusTok/logger"                    // 日志
	"github.com/c1cada/NexusTok/setting/operation_setting" // 运营设置

	"github.com/bytedance/gopkg/util/gopool" // 协程池
	"gorm.io/gorm"                           // GORM ORM
)

// UserNameMaxLength 用户名最大长度
const UserNameMaxLength = 20

// User 用户数据模型
// 注意：如果添加敏感字段，不要忘记在 setupLogin 函数中清理
// 否则敏感信息会以明文形式保存在本地存储中！
type User struct {
	Id               int                        `json:"id"`                                                                                                // 用户 ID
	Username         string                     `json:"username" gorm:"unique;index" validate:"max=20"`                                                    // 用户名（唯一索引）
	Password         string                     `json:"password" gorm:"not null;" validate:"min=8,max=20"`                                                 // 密码（哈希后存储）
	OriginalPassword string                     `json:"original_password" gorm:"-:all"`                                                                    // 原始密码（仅用于密码修改验证，不存储到数据库）
	DisplayName      string                     `json:"display_name" gorm:"index" validate:"max=20"`                                                       // 显示名称
	Role             int                        `json:"role" gorm:"type:int;default:1"`                                                                    // 用户角色（1=普通用户，10=管理员，100=Root）
	AuthzRole        string                     `json:"authz_role,omitempty" gorm:"type:varchar(64);column:authz_role;default:'';index" validate:"max=64"` // 管理员授权角色模板；为空表示使用内置 Admin 基线
	Status           int                        `json:"status" gorm:"type:int;default:1"`                                                                  // 用户状态（1=启用，2=禁用）
	Email            string                     `json:"email" gorm:"index" validate:"max=50"`                                                              // 邮箱地址
	GitHubId         string                     `json:"github_id" gorm:"column:github_id;index"`                                                           // GitHub ID（OAuth 关联）
	DiscordId        string                     `json:"discord_id" gorm:"column:discord_id;index"`                                                         // Discord ID（OAuth 关联）
	OidcId           string                     `json:"oidc_id" gorm:"column:oidc_id;index"`                                                               // OIDC ID（OAuth 关联）
	WeChatId         string                     `json:"wechat_id" gorm:"column:wechat_id;index"`                                                           // 微信 ID（OAuth 关联）
	TelegramId       string                     `json:"telegram_id" gorm:"column:telegram_id;index"`                                                       // Telegram ID（OAuth 关联）
	VerificationCode string                     `json:"verification_code" gorm:"-:all"`                                                                    // 邮箱验证码（仅用于邮箱验证，不存储到数据库）
	AccessToken      *string                    `json:"access_token" gorm:"type:char(32);column:access_token;uniqueIndex"`                                 // 系统管理用 Access Token
	Quota            int                        `json:"quota" gorm:"type:int;default:0"`                                                                   // 用户总额度
	UsedQuota        int                        `json:"used_quota" gorm:"type:int;default:0;column:used_quota"`                                            // 已使用额度
	RequestCount     int                        `json:"request_count" gorm:"type:int;default:0;"`                                                          // 请求次数
	Group            string                     `json:"group" gorm:"type:varchar(64);default:'default'"`                                                   // 用户分组
	AffCode          string                     `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`                                      // 邀请码
	AffCount         int                        `json:"aff_count" gorm:"type:int;default:0;column:aff_count"`                                              // 邀请人数
	AffQuota         int                        `json:"aff_quota" gorm:"type:int;default:0;column:aff_quota"`                                              // 邀请剩余额度
	AffHistoryQuota  int                        `json:"aff_history_quota" gorm:"type:int;default:0;column:aff_history"`                                    // 邀请历史额度
	InviterId        int                        `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`                                                // 邀请人 ID
	DeletedAt        gorm.DeletedAt             `gorm:"index"`                                                                                             // 软删除时间
	LinuxDOId        string                     `json:"linux_do_id" gorm:"column:linux_do_id;index"`                                                       // Linux DO ID（OAuth 关联）
	Setting          string                     `json:"setting" gorm:"type:text;column:setting"`                                                           // 用户设置（JSON 格式）
	Remark           string                     `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`                                      // 备注
	StripeCustomer   string                     `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`                              // Stripe 客户 ID
	CreatedAt        int64                      `json:"created_at" gorm:"autoCreateTime;column:created_at"`                                                // 创建时间
	LastLoginAt      int64                      `json:"last_login_at" gorm:"default:0;column:last_login_at"`                                               // 最后登录时间
	AdminPermissions map[string]map[string]bool `json:"admin_permissions,omitempty" gorm:"-:all"`                                                          // 管理权限矩阵，仅用于 API payload，不写入 users 表
}

// ToBaseUser 将用户转换为基础用户信息（用于缓存）
// 基础用户信息只包含请求处理所需的关键字段
//
// 返回值：
//   - *UserBase: 基础用户信息
func (user *User) ToBaseUser() *UserBase {
	cache := &UserBase{
		Id:       user.Id,
		Group:    user.Group,
		Quota:    user.Quota,
		Status:   user.Status,
		Username: user.Username,
		Setting:  user.Setting,
		Email:    user.Email,
	}
	return cache
}

// GetAccessToken 获取用户的 Access Token
// 如果 Access Token 为空，返回空字符串
//
// 返回值：
//   - string: Access Token
func (user *User) GetAccessToken() string {
	if user.AccessToken == nil {
		return ""
	}
	return *user.AccessToken
}

// SetAccessToken 设置用户的 Access Token
//
// 参数：
//   - token: Access Token
func (user *User) SetAccessToken(token string) {
	user.AccessToken = &token
}

// GetSetting 获取用户的设置
// 从 JSON 格式的设置字符串中解析出 UserSetting 对象
//
// 返回值：
//   - dto.UserSetting: 用户设置对象
func (user *User) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := common.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		}
	}
	return setting
}

// SetSetting 设置用户的设置
// 将 UserSetting 对象序列化为 JSON 格式存储
//
// 参数：
//   - setting: 用户设置对象
func (user *User) SetSetting(setting dto.UserSetting) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog("failed to marshal setting: " + err.Error())
		return
	}
	user.Setting = string(settingBytes)
}

// UpdateUserSetting 只更新用户设置列，并同步刷新设置缓存。
//
// 个人设置保存经常发生在用户请求和计费扣减并发运行时，因此这里不能复用
// User.Update 的整行更新语义。该函数只写入 setting 字段，保证 quota、
// used_quota、request_count 等账务字段不会被旧用户快照覆盖。
func UpdateUserSetting(userId int, setting dto.UserSetting) error {
	if userId == 0 {
		return errors.New("id 为空！")
	}
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		return err
	}
	settingValue := string(settingBytes)
	if err = DB.Model(&User{}).Where("id = ?", userId).Update("setting", settingValue).Error; err != nil {
		return err
	}
	return updateUserSettingCache(userId, settingValue)
}

// generateDefaultSidebarConfigForRole 根据用户角色生成默认的边栏配置
// 不同角色的用户可以看到不同的菜单项
//
// 角色权限：
// - 普通用户：聊天、控制台、个人中心
// - 管理员：普通用户功能 + 管理员区域（不含系统设置）
// - Root 用户：所有功能
//
// 参数：
//   - userRole: 用户角色
//
// 返回值：
//   - string: JSON 格式的边栏配置
func generateDefaultSidebarConfigForRole(userRole int) string {
	defaultConfig := map[string]interface{}{}

	// 聊天区域 - 所有用户都可以访问
	defaultConfig["chat"] = map[string]interface{}{
		"enabled":    true,
		"playground": true,
		"chat":       true,
	}

	// 控制台区域 - 所有用户都可以访问
	defaultConfig["console"] = map[string]interface{}{
		"enabled":    true,
		"detail":     true,
		"token":      true,
		"log":        true,
		"midjourney": true,
		"task":       true,
	}

	// 个人中心区域 - 所有用户都可以访问
	defaultConfig["personal"] = map[string]interface{}{
		"enabled":  true,
		"topup":    true,
		"personal": true,
	}

	// 管理员区域 - 根据角色决定
	if userRole == common.RoleAdminUser {
		// 管理员可以访问管理员区域，但不能访问系统设置
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":      true,
			"channel":      true,
			"account_pool": true,
			"models":       true,
			"redemption":   true,
			"user":         true,
			"setting":      false, // 管理员不能访问系统设置
		}
	} else if userRole == common.RoleRootUser {
		// 超级管理员可以访问所有功能
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":      true,
			"channel":      true,
			"account_pool": true,
			"models":       true,
			"redemption":   true,
			"user":         true,
			"setting":      true,
		}
	}
	// 普通用户不包含admin区域

	// 转换为JSON字符串
	configBytes, err := common.Marshal(defaultConfig)
	if err != nil {
		common.SysLog("生成默认边栏配置失败: " + err.Error())
		return ""
	}

	return string(configBytes)
}

// CheckUserExistOrDeleted 检查用户名或邮箱是否已存在（包含软删除记录）。
//
// 返回值含义：
//   - false, nil：用户名和邮箱均未被占用。
//   - true, nil：存在正常或软删除用户占用了用户名/邮箱。
//   - false, error：数据库查询失败。
func CheckUserExistOrDeleted(username string, email string) (bool, error) {
	var user User

	var err error
	email = NormalizeEmail(email)
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ?", username).Error
	} else {
		err = DB.Unscoped().First(&user, "username = ? or LOWER(email) = ?", username, email).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// not exist, return false, nil
			return false, nil
		}
		// other error, return false, err
		return false, err
	}
	// exist, return true, nil
	return true, nil
}

// NormalizeEmail 将邮箱转换为模型层持久化和比较所用的规范格式。
//
// 邮箱在业务语义上需要大小写不敏感，历史数据中又可能存在不同大小写的
// 重复值。所有注册、绑定、登录辅助查询和密码重置入口都应先调用该函数，
// 再进入数据库比较，避免同一邮箱因为大小写或首尾空白被当成多个账号。
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// emailQuery 返回大小写不敏感的邮箱查询。
//
// 查询包含软删除记录，用于注册和绑定前的唯一性检查；这样被软删除的历史
// 账号不会被新账号悄悄复用邮箱，后续恢复账号时也不会产生冲突。
func emailQuery(tx *gorm.DB, email string) *gorm.DB {
	if tx == nil {
		tx = DB
	}
	return tx.Unscoped().Model(&User{}).Where("LOWER(email) = ?", NormalizeEmail(email))
}

// CountUsersByEmail 统计规范化邮箱匹配到的用户数量。
func CountUsersByEmail(email string) (int64, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return 0, nil
	}
	var count int64
	err := emailQuery(DB, email).Count(&count).Error
	return count, err
}

// IsEmailAvailable 判断邮箱是否可用于指定用户。
//
// excludeUserID 用于用户修改自身邮箱时排除当前账号；传 0 表示不能被任何
// 历史账号占用。空邮箱允许重复，兼容无邮箱的 OAuth/passwordless 用户。
func IsEmailAvailable(email string, excludeUserID int) (bool, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return true, nil
	}
	query := emailQuery(DB, email)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

// EnsureEmailAvailable 在邮箱不可用时返回稳定的模型层错误。
func EnsureEmailAvailable(email string, excludeUserID int) error {
	available, err := IsEmailAvailable(email, excludeUserID)
	if err != nil {
		return err
	}
	if !available {
		return ErrEmailAlreadyTaken
	}
	return nil
}

// ensureEmailAvailableWithTx 在指定事务中执行邮箱唯一性检查。
func ensureEmailAvailableWithTx(tx *gorm.DB, email string, excludeUserID int) error {
	email = NormalizeEmail(email)
	if email == "" {
		return nil
	}
	query := emailQuery(tx, email)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrEmailAlreadyTaken
	}
	return nil
}

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

func GetAllUsers(pageInfo *common.PageInfo) (users []*User, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	err = tx.Unscoped().Model(&User{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated users within same transaction
	err = tx.Unscoped().Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("password").Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func SearchUsers(keyword string, group string, startIdx int, num int) ([]*User, int64, error) {
	var users []*User
	var total int64
	var err error

	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 构建基础查询
	query := tx.Unscoped().Model(&User{})

	// 构建搜索条件
	likeCondition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"

	// 尝试将关键字转换为整数ID
	keywordInt, err := strconv.Atoi(keyword)
	if err == nil {
		// 如果是数字，同时搜索ID和其他字段
		likeCondition = "id = ? OR " + likeCondition
		if group != "" {
			query = query.Where("("+likeCondition+") AND "+commonGroupCol+" = ?",
				keywordInt, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", group)
		} else {
			query = query.Where(likeCondition,
				keywordInt, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
		}
	} else {
		// 非数字关键字，只搜索字符串字段
		if group != "" {
			query = query.Where("("+likeCondition+") AND "+commonGroupCol+" = ?",
				"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", group)
		} else {
			query = query.Where(likeCondition,
				"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
		}
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Omit("password").Order("id desc").Limit(num).Offset(startIdx).Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func GetUserById(id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	user := User{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(&user, "id = ?", id).Error
	} else {
		err = DB.Omit("password").First(&user, "id = ?", id).Error
	}
	return &user, err
}

func GetUserIdByAffCode(affCode string) (int, error) {
	if affCode == "" {
		return 0, errors.New("affCode 为空！")
	}
	var user User
	err := DB.Select("id").First(&user, "aff_code = ?", affCode).Error
	return user.Id, err
}

func DeleteUserById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.Delete()
}

func HardDeleteUserById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Delete(&User{}, "id = ?", id).Error; err != nil {
			return err
		}
		return ClearAuthzUserOverridesInTx(tx, id)
	})
}

func inviteUser(inviterId int) (err error) {
	user, err := GetUserById(inviterId, true)
	if err != nil {
		return err
	}
	user.AffCount++
	user.AffQuota += common.QuotaForInviter
	user.AffHistoryQuota += common.QuotaForInviter
	return DB.Save(user).Error
}

func (user *User) TransferAffQuotaToQuota(quota int) error {
	// 检查quota是否小于最小额度
	if float64(quota) < common.QuotaPerUnit {
		return fmt.Errorf("转移额度最小为%s！", logger.LogQuota(int(common.QuotaPerUnit)))
	}

	// 开始数据库事务
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback() // 确保在函数退出时事务能回滚

	// 加锁查询用户以确保数据一致性
	err := lockForUpdate(tx).First(&user, user.Id).Error
	if err != nil {
		return err
	}

	// 再次检查用户的AffQuota是否足够
	if user.AffQuota < quota {
		return errors.New("邀请额度不足！")
	}

	// 更新用户额度
	user.AffQuota -= quota
	user.Quota += quota

	// 保存用户状态
	if err := tx.Save(user).Error; err != nil {
		return err
	}

	// 提交事务
	return tx.Commit().Error
}

// prepareForInsert 统一处理用户写入前的规范化与安全检查。
//
// 该函数只在 Insert/InsertWithTx 中调用：邮箱先转换为规范格式，再在同一
// 事务内执行大小写不敏感唯一性检查；密码只有在非空时才哈希，确保 OAuth
// 或 Passkey 等 passwordless 用户能够继续以空密码形式保存。
func (user *User) prepareForInsert(tx *gorm.DB) error {
	user.Email = NormalizeEmail(user.Email)
	if err := ensureEmailAvailableWithTx(tx, user.Email, 0); err != nil {
		return err
	}
	if user.Password == "" {
		return nil
	}
	var err error
	user.Password, err = common.Password2Hash(user.Password)
	return err
}

func (user *User) Insert(inviterId int) error {
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := user.prepareForInsert(tx); err != nil {
			return err
		}
		user.Quota = common.QuotaForNewUser
		//user.SetAccessToken(common.GetUUID())
		user.AffCode = common.GetRandomString(4)

		// 初始化用户设置，包括默认的边栏配置
		if user.Setting == "" {
			defaultSetting := dto.UserSetting{}
			// 这里暂时不设置SidebarModules，因为需要在用户创建后根据角色设置
			user.SetSetting(defaultSetting)
		}

		return tx.Create(user).Error
	}); err != nil {
		return err
	}

	user.FinalizeUserCreation(inviterId)
	return nil
}

// InsertWithTx 在外部事务中创建用户。
//
// OAuth 注册需要把用户创建和第三方绑定放在同一个事务内，确保中间任一
// 步失败时不会留下半绑定账号。侧边栏初始化、注册日志和邀请奖励等写入
// 依赖已提交的用户 ID，应在事务提交成功后由调用方执行。
func (user *User) InsertWithTx(tx *gorm.DB, inviterId int) error {
	if tx == nil {
		tx = DB
	}
	if err := user.prepareForInsert(tx); err != nil {
		return err
	}
	user.Quota = common.QuotaForNewUser
	user.AffCode = common.GetRandomString(4)

	// 初始化用户设置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		user.SetSetting(defaultSetting)
	}

	result := tx.Create(user)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// FinalizeUserCreation 执行用户创建事务提交后的收尾任务。
//
// 该函数必须在外部事务成功提交后调用，用于初始化侧边栏配置、记录注册赠额日志和处理
// 邀请奖励。它不参与创建事务，避免侧栏设置、日志或邀请奖励失败时反向影响已经完成的
// 账号创建；调用方只需要保证 user.Id 已经由 GORM 回填。
func (user *User) FinalizeUserCreation(inviterId int) {
	// 用户创建成功后，根据角色初始化边栏配置
	var createdUser User
	if err := DB.Where("id = ?", user.Id).First(&createdUser).Error; err == nil {
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			createdUser.SetSetting(currentSetting)
			createdUser.Update(false)
			common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", logger.LogQuota(common.QuotaForNewUser)))
	}
	if inviterId != 0 && operation_setting.IsPaymentComplianceConfirmed() {
		if common.QuotaForInvitee > 0 {
			_ = IncreaseUserQuota(user.Id, common.QuotaForInvitee, true)
			RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", logger.LogQuota(common.QuotaForInvitee)))
		}
		if common.QuotaForInviter > 0 {
			RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请用户赠送 %s", logger.LogQuota(common.QuotaForInviter)))
			_ = inviteUser(inviterId)
		}
	}
}

// FinalizeOAuthUserCreation 保留 OAuth 注册路径使用的历史函数名。
//
// OAuth 和后台创建用户现在共享同一套创建后收尾逻辑；保留这个包装函数可以避免旧调用点
// 大范围重命名，同时让函数名里的 OAuth 语义不再泄漏到普通后台创建流程。
func (user *User) FinalizeOAuthUserCreation(inviterId int) {
	user.FinalizeUserCreation(inviterId)
}

func (user *User) Update(updatePassword bool) error {
	if err := user.UpdateWithTx(DB, updatePassword); err != nil {
		return err
	}

	// 更新缓存
	return updateUserCache(*user)
}

// UpdateWithTx 在事务中更新用户资料，并保护账务字段不被旧快照覆盖。
//
// 用户对象常由控制器先读取再修改局部字段后保存。在这段时间里，relay
// 计费、兑换、订阅或管理员额度操作可能已经更新了 quota、used_quota 和
// request_count。这里显式 Omit 这三个字段，并在更新后重新读取当前行，
// 保证调用方拿到的是数据库最新状态。
func (user *User) UpdateWithTx(tx *gorm.DB, updatePassword bool) error {
	if tx == nil {
		tx = DB
	}
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	newUser := *user
	current := User{}
	if err = tx.First(&current, user.Id).Error; err != nil {
		return err
	}
	if err = tx.Model(&current).Omit("quota", "used_quota", "request_count").Updates(newUser).Error; err != nil {
		return err
	}
	return tx.First(user, user.Id).Error
}

func (user *User) Edit(updatePassword bool) error {
	if err := user.EditWithTx(DB, updatePassword); err != nil {
		return err
	}

	// 更新缓存
	return updateUserCache(*user)
}

// EditWithTx 在事务中更新管理员可编辑的用户资料字段。
//
// 用户管理页的资料保存可能与权限 override 保存放在同一事务里执行，因此这里只
// 更新明确允许的资料字段：用户名、显示名、分组、备注和可选密码。额度、已用量、
// 请求次数、角色和第三方绑定等字段不在该路径写入，避免资料编辑覆盖并发业务状态。
func (user *User) EditWithTx(tx *gorm.DB, updatePassword bool) error {
	if tx == nil {
		tx = DB
	}
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}

	newUser := *user
	updates := map[string]interface{}{
		"username":     newUser.Username,
		"display_name": newUser.DisplayName,
		"group":        newUser.Group,
		"remark":       newUser.Remark,
	}
	if updatePassword {
		updates["password"] = newUser.Password
	}

	if err = tx.First(user, user.Id).Error; err != nil {
		return err
	}
	if err = tx.Model(user).Updates(updates).Error; err != nil {
		return err
	}
	return tx.First(user, user.Id).Error
}

// BindEmailToUser 将规范化邮箱绑定到指定用户。
//
// 绑定前会排除当前用户并检查邮箱是否已被其他历史账号占用。检查和更新在
// 同一事务中完成，避免控制器在验证码校验成功后绕过模型层唯一性规则。
func BindEmailToUser(user *User, email string) error {
	email = NormalizeEmail(email)
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureEmailAvailableWithTx(tx, email, user.Id); err != nil {
			return err
		}
		user.Email = email
		return user.UpdateWithTx(tx, false)
	}); err != nil {
		return err
	}
	return updateUserCache(*user)
}

func (user *User) ClearBinding(bindingType string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}

	bindingColumnMap := map[string]string{
		"email":    "email",
		"github":   "github_id",
		"discord":  "discord_id",
		"oidc":     "oidc_id",
		"wechat":   "wechat_id",
		"telegram": "telegram_id",
		"linuxdo":  "linux_do_id",
	}

	column, ok := bindingColumnMap[bindingType]
	if !ok {
		return errors.New("invalid binding type")
	}

	if err := DB.Model(&User{}).Where("id = ?", user.Id).Update(column, "").Error; err != nil {
		return err
	}

	if err := DB.Where("id = ?", user.Id).First(user).Error; err != nil {
		return err
	}

	return updateUserCache(*user)
}

func (user *User) Delete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(user).Error; err != nil {
			return err
		}
		return ClearAuthzUserOverridesInTx(tx, user.Id)
	}); err != nil {
		return err
	}

	// 清除缓存
	return invalidateUserCache(user.Id)
}

func (user *User) HardDelete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Delete(user).Error; err != nil {
			return err
		}
		return ClearAuthzUserOverridesInTx(tx, user.Id)
	})
}

// ValidateAndFill 校验密码登录凭据并填充用户信息。
func (user *User) ValidateAndFill() (err error) {
	// 这里必须显式记录明文密码并用条件查询用户；如果直接用结构体查询，
	// GORM 会忽略空字符串、0、false 等零值字段，容易构造出不完整条件。
	password := user.Password
	username := strings.TrimSpace(user.Username)
	if username == "" || password == "" {
		return ErrUserEmptyCredentials
	}
	// 支持用户名或邮箱登录；邮箱比较使用规范化值，兼容用户输入大小写不同
	// 的邮箱地址，同时不改变用户名登录的精确匹配语义。
	err = DB.Where("username = ? OR LOWER(email) = ?", username, NormalizeEmail(username)).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Join(ErrInvalidCredentials, ErrLoginUserNotFound)
		}
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if user.Password == "" {
		return errors.Join(ErrInvalidCredentials, ErrLoginEmptyPasswordHash)
	}
	okay := common.ValidatePasswordAndHash(password, user.Password)
	if !okay {
		return errors.Join(ErrInvalidCredentials, ErrLoginPasswordMismatch)
	}
	if user.Status != common.UserStatusEnabled {
		return errors.Join(ErrInvalidCredentials, ErrLoginUserDisabled)
	}
	return nil
}

func (user *User) FillUserById() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	DB.Where(User{Id: user.Id}).First(user)
	return nil
}

func (user *User) FillUserByEmail() error {
	if user.Email == "" {
		return errors.New("email 为空！")
	}
	user.Email = NormalizeEmail(user.Email)
	DB.Where("LOWER(email) = ?", user.Email).First(user)
	return nil
}

func (user *User) FillUserByGitHubId() error {
	if user.GitHubId == "" {
		return errors.New("GitHub id 为空！")
	}
	DB.Where(User{GitHubId: user.GitHubId}).First(user)
	return nil
}

// UpdateGitHubId updates the user's GitHub ID (used for migration from login to numeric ID)
func (user *User) UpdateGitHubId(newGitHubId string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}
	return DB.Model(user).Update("github_id", newGitHubId).Error
}

func (user *User) FillUserByDiscordId() error {
	if user.DiscordId == "" {
		return errors.New("discord id 为空！")
	}
	DB.Where(User{DiscordId: user.DiscordId}).First(user)
	return nil
}

func (user *User) FillUserByOidcId() error {
	if user.OidcId == "" {
		return errors.New("oidc id 为空！")
	}
	DB.Where(User{OidcId: user.OidcId}).First(user)
	return nil
}

func (user *User) FillUserByWeChatId() error {
	if user.WeChatId == "" {
		return errors.New("WeChat id 为空！")
	}
	DB.Where(User{WeChatId: user.WeChatId}).First(user)
	return nil
}

func (user *User) FillUserByTelegramId() error {
	if user.TelegramId == "" {
		return errors.New("Telegram id 为空！")
	}
	err := DB.Where(User{TelegramId: user.TelegramId}).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("该 Telegram 账户未绑定")
	}
	return nil
}

func IsEmailAlreadyTaken(email string) bool {
	count, err := CountUsersByEmail(email)
	return err == nil && count > 0
}

// GetUniqueUserByEmail 按规范化邮箱查找唯一用户。
//
// 密码重置等高风险路径必须使用该函数。若历史数据中存在大小写不同的重复
// 邮箱，函数返回 ErrEmailAmbiguous，调用方应拒绝自动执行写操作并提示用户
// 重新发起或联系管理员处理历史数据。
func GetUniqueUserByEmail(email string) (*User, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return nil, ErrEmailNotFound
	}
	var users []User
	if err := DB.Where("LOWER(email) = ?", email).Limit(2).Find(&users).Error; err != nil {
		return nil, err
	}
	switch len(users) {
	case 0:
		return nil, ErrEmailNotFound
	case 1:
		return &users[0], nil
	default:
		return nil, ErrEmailAmbiguous
	}
}

func IsWeChatIdAlreadyTaken(wechatId string) bool {
	return DB.Unscoped().Where("wechat_id = ?", wechatId).Find(&User{}).RowsAffected == 1
}

func IsGitHubIdAlreadyTaken(githubId string) bool {
	return DB.Unscoped().Where("github_id = ?", githubId).Find(&User{}).RowsAffected == 1
}

func IsDiscordIdAlreadyTaken(discordId string) bool {
	return DB.Unscoped().Where("discord_id = ?", discordId).Find(&User{}).RowsAffected == 1
}

func IsOidcIdAlreadyTaken(oidcId string) bool {
	return DB.Where("oidc_id = ?", oidcId).Find(&User{}).RowsAffected == 1
}

func IsTelegramIdAlreadyTaken(telegramId string) bool {
	return DB.Unscoped().Where("telegram_id = ?", telegramId).Find(&User{}).RowsAffected == 1
}

func ResetUserPasswordByEmail(email string, password string) error {
	if email == "" || password == "" {
		return errors.New("邮箱地址或密码为空！")
	}
	user, err := GetUniqueUserByEmail(email)
	if err != nil {
		return err
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	err = DB.Model(&User{}).Where("id = ?", user.Id).Update("password", hashedPassword).Error
	return err
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		common.SysLog("no such user " + err.Error())
		return false
	}
	return user.Role >= common.RoleAdminUser
}

//// IsUserEnabled checks user status from Redis first, falls back to DB if needed
//func IsUserEnabled(id int, fromDB bool) (status bool, err error) {
//	defer func() {
//		// Update Redis cache asynchronously on successful DB read
//		if shouldUpdateRedis(fromDB, err) {
//			gopool.Go(func() {
//				if err := updateUserStatusCache(id, status); err != nil {
//					common.SysError("failed to update user status cache: " + err.Error())
//				}
//			})
//		}
//	}()
//	if !fromDB && common.RedisEnabled {
//		// Try Redis first
//		status, err := getUserStatusCache(id)
//		if err == nil {
//			return status == common.UserStatusEnabled, nil
//		}
//		// Don't return error - fall through to DB
//	}
//	fromDB = true
//	var user User
//	err = DB.Where("id = ?", id).Select("status").Find(&user).Error
//	if err != nil {
//		return false, err
//	}
//
//	return user.Status == common.UserStatusEnabled, nil
//}

func ValidateAccessToken(token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user := &User{}
	err := DB.Where("access_token = ?", token).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return user, nil
}

// GetUserQuota gets quota from Redis first, falls back to DB if needed
func GetUserQuota(id int, fromDB bool) (quota int, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserQuotaCache(id, quota); err != nil {
					common.SysLog("failed to update user quota cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		quota, err := getUserQuotaCache(id)
		if err == nil {
			return quota, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error
	if err != nil {
		return 0, err
	}

	return quota, nil
}

func GetUserUsedQuota(id int) (quota int, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("used_quota").Find(&quota).Error
	return quota, err
}

func GetUserEmail(id int) (email string, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("email").Find(&email).Error
	return email, err
}

// GetUserGroup gets group from Redis first, falls back to DB if needed
func GetUserGroup(id int, fromDB bool) (group string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserGroupCache(id, group); err != nil {
					common.SysLog("failed to update user group cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		group, err := getUserGroupCache(id)
		if err == nil {
			return group, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select(commonGroupCol).Find(&group).Error
	if err != nil {
		return "", err
	}

	return group, nil
}

// GetUserSetting gets setting from Redis first, falls back to DB if needed
func GetUserSetting(id int, fromDB bool) (settingMap dto.UserSetting, err error) {
	var setting string
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserSettingCache(id, setting); err != nil {
					common.SysLog("failed to update user setting cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		setting, err := getUserSettingCache(id)
		if err == nil {
			return setting, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	// can be nil setting
	var safeSetting sql.NullString
	err = DB.Model(&User{}).Where("id = ?", id).Select("setting").Find(&safeSetting).Error
	if err != nil {
		return settingMap, err
	}
	if safeSetting.Valid {
		setting = safeSetting.String
	} else {
		setting = ""
	}
	userBase := &UserBase{
		Setting: setting,
	}
	return userBase.GetSetting(), nil
}

func IncreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	gopool.Go(func() {
		err := cacheIncrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to increase user quota: " + err.Error())
		}
	})
	if !db && common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, quota)
		return nil
	}
	return increaseUserQuota(id, quota)
}

func increaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func DecreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	gopool.Go(func() {
		err := cacheDecrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to decrease user quota: " + err.Error())
		}
	})
	if !db && common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, -quota)
		return nil
	}
	return decreaseUserQuota(id, quota)
}

func decreaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota - ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func DeltaUpdateUserQuota(id int, delta int) (err error) {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return IncreaseUserQuota(id, delta, false)
	} else {
		return DecreaseUserQuota(id, -delta, false)
	}
}

//func GetRootUserEmail() (email string) {
//	DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Select("email").Find(&email)
//	return email
//}

func GetRootUser() (user *User) {
	DB.Where("role = ?", common.RoleRootUser).First(&user)
	return user
}

func UpdateUserLastLoginAt(id int) {
	if err := DB.Model(&User{}).Where("id = ?", id).Update("last_login_at", common.GetTimestamp()).Error; err != nil {
		common.SysLog("failed to update user last_login_at: " + err.Error())
	}
}

func UpdateUserUsedQuotaAndRequestCount(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	updateUserUsedQuotaAndRequestCount(id, quota, 1)
}

func updateUserUsedQuotaAndRequestCount(id int, quota int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"request_count": gorm.Expr("request_count + ?", count),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota and request count: " + err.Error())
		return
	}

	//// 更新缓存
	//if err := invalidateUserCache(id); err != nil {
	//	common.SysError("failed to invalidate user cache: " + err.Error())
	//}
}

func updateUserUsedQuota(id int, quota int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", quota),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota: " + err.Error())
	}
}

func updateUserRequestCount(id int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", count)).Error
	if err != nil {
		common.SysLog("failed to update user request count: " + err.Error())
	}
}

// GetUsernameById gets username from Redis first, falls back to DB if needed
func GetUsernameById(id int, fromDB bool) (username string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserNameCache(id, username); err != nil {
					common.SysLog("failed to update user name cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		username, err := getUserNameCache(id)
		if err == nil {
			return username, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("username").Find(&username).Error
	if err != nil {
		return "", err
	}

	return username, nil
}

func IsLinuxDOIdAlreadyTaken(linuxDOId string) bool {
	var user User
	err := DB.Unscoped().Where("linux_do_id = ?", linuxDOId).First(&user).Error
	return !errors.Is(err, gorm.ErrRecordNotFound)
}

func (user *User) FillUserByLinuxDOId() error {
	if user.LinuxDOId == "" {
		return errors.New("linux do id is empty")
	}
	err := DB.Where("linux_do_id = ?", user.LinuxDOId).First(user).Error
	return err
}

func RootUserExists() bool {
	var user User
	err := DB.Where("role = ?", common.RoleRootUser).First(&user).Error
	if err != nil {
		return false
	}
	return true
}
