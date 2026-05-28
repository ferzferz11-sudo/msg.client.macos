// Lavender Messenger - A secure messaging application
// Author: Pavel Davydov (ferz)
//
// This file implements a macOS GUI client for the Lavender Messenger.
// It provides a graphical interface using Fyne framework with themes and emojis.

package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"LavenderMessenger/gen"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

const clientVersion = "1.1.0.0"

var myWindow fyne.Window
var chatBox *widget.RichText
var connectToServer func(string)
var avatarCache = make(map[string]string) // Cache for avatar URLs
var avatarCacheMutex sync.Mutex           // Mutex for thread-safe avatarCache access
var loginForm *dialog.ConfirmDialog       // Global reference to login form for re-showing after registration
var globalCfg *Config                     // Global config reference for registration
var currentUsername string                // Current logged-in username
var currentPassword string                // Current logged-in password
var currentConn *grpc.ClientConn          // Current gRPC connection

// Server list from GetServers RPC
type ServerEntry struct {
	Name    string
	Address string
}
var serverList []ServerEntry // Populated by fetchServers

// fetchServers loads server list from GetServers RPC
func fetchServers(defaultAddr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(defaultAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		// Fallback to default servers if RPC fails
		serverList = []ServerEntry{
			{Name: "Новый", Address: "13.140.25.249:50051"},
			{Name: "Старый", Address: "159.195.38.145:50051"},
		}
		return
	}
	defer conn.Close()

	client := gen.NewServerServiceClient(conn)
	resp, err := client.GetServers(ctx, &gen.GetServersRequest{})
	if err != nil || len(resp.Servers) == 0 {
		// Fallback
		serverList = []ServerEntry{
			{Name: "Новый", Address: "13.140.25.249:50051"},
			{Name: "Старый", Address: "159.195.38.145:50051"},
		}
		return
	}

	var servers []ServerEntry
	for _, s := range resp.Servers {
		addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
		name := s.Name
		if name == "" {
			name = addr
		}
		if s.IsDefault {
			name = name + " (default)"
			// Put default first
			servers = append([]ServerEntry{{Name: name, Address: addr}}, servers...)
		} else {
			servers = append(servers, ServerEntry{Name: name, Address: addr})
		}
	}
	serverList = servers
}

func getConfigPaths() []string {
	var paths []string
	// Сначала проверяем директорию, где находится main.go
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		dir := filepath.Dir(filename)
		paths = append(paths, filepath.Join(dir, "config.yaml"))
	}
	// Затем проверяем текущую директорию
	paths = append(paths, "config.yaml")
	// И другие возможные пути
	paths = append(paths, "client/macos/config.yaml")
	paths = append(paths, "../macos/config.yaml")
	return paths
}

type Config struct {
	ServerAddress string `yaml:"server_address"`
	Themes        struct {
		Light ThemeConfig `yaml:"light"`
		Dark  ThemeConfig `yaml:"dark"`
	} `yaml:"themes"`
	CurrentTheme string `yaml:"current_theme"`
	LastUsername string `yaml:"last_username"`
	LastPassword string `yaml:"last_password"`
	LastEmail    string `yaml:"last_email"`
}

type ThemeConfig struct {
	BgColor    string   `yaml:"bg_color"`
	TextColor  string   `yaml:"text_color"`
	NameColor  string   `yaml:"name_color"`
	TimeColor  string   `yaml:"time_color"`
	UserColors []string `yaml:"user_colors"`
}

// loadConfig загружает конфигурацию из YAML файла
func loadConfig() (*Config, error) {
	paths := getConfigPaths()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			var cfg Config
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				continue
			}
			return &cfg, nil
		}
	}
	return nil, fmt.Errorf("config not found")
}

// saveConfig сохраняет конфигурацию в YAML файл
func saveConfig(cfg *Config) error {
	// Try to save to first writable path
	paths := getConfigPaths()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := os.WriteFile(path, data, 0644); err == nil {
			return nil
		}
	}
	return fmt.Errorf("could not write config to any path")
}

// Вспомогательная функция для парсинга HEX в color.RGBA
func parseHexColor(s string) color.Color {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.Transparent
	}
	var r, g, b uint8
	_, _ = fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// Кастомные имена цветов для пользователей
const (
	UserColor1  fyne.ThemeColorName = "UserColor1"
	UserColor2  fyne.ThemeColorName = "UserColor2"
	UserColor3  fyne.ThemeColorName = "UserColor3"
	UserColor4  fyne.ThemeColorName = "UserColor4"
	UserColor5  fyne.ThemeColorName = "UserColor5"
	UserColor6  fyne.ThemeColorName = "UserColor6"
	UserColor7  fyne.ThemeColorName = "UserColor7"
	UserColor8  fyne.ThemeColorName = "UserColor8"
	UserColor9  fyne.ThemeColorName = "UserColor9"
	UserColor10 fyne.ThemeColorName = "UserColor10"
)

// customTheme для поддержки своих цветов
type customTheme struct {
	isDark                    bool
	lightBg, darkBg           color.Color
	lightFg, darkFg           color.Color
	lightPrimary, darkPrimary color.Color
	lightTime, darkTime       color.Color
	userColors                []color.Color
}

func (c *customTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameBackground {
		if c.isDark {
			return c.darkBg
		}
		return c.lightBg
	}
	if name == theme.ColorNameForeground {
		if c.isDark {
			return c.darkFg
		}
		return c.lightFg
	}
	if name == theme.ColorNamePrimary {
		if c.isDark {
			return c.darkPrimary
		}
		return c.lightPrimary
	}
	if name == theme.ColorNameDisabled {
		if c.isDark {
			return c.darkTime
		}
		return c.lightTime
	}

	// Обработка кастомных цветов пользователей
	if len(c.userColors) > 0 {
		switch name {
		case UserColor1:
			return c.userColors[0%len(c.userColors)]
		case UserColor2:
			return c.userColors[1%len(c.userColors)]
		case UserColor3:
			return c.userColors[2%len(c.userColors)]
		case UserColor4:
			return c.userColors[3%len(c.userColors)]
		case UserColor5:
			return c.userColors[4%len(c.userColors)]
		case UserColor6:
			return c.userColors[5%len(c.userColors)]
		case UserColor7:
			return c.userColors[6%len(c.userColors)]
		case UserColor8:
			return c.userColors[7%len(c.userColors)]
		case UserColor9:
			return c.userColors[8%len(c.userColors)]
		case UserColor10:
			return c.userColors[9%len(c.userColors)]
		}
	}

	// Для остальных используем дефолтную тему
	if c.isDark {
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
	return theme.DefaultTheme().Color(name, theme.VariantLight)
}

func (c *customTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (c *customTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (c *customTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}

// getChats получает список чатов пользователя
func getChats(client gen.ChatServiceClient, username string) ([]*gen.ChatInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetChats(ctx, &gen.GetChatsRequest{Username: username})
	if err != nil {
		return nil, err
	}
	return resp.Chats, nil
}

// getUserAvatar loads user avatar from server
func getUserAvatar(client gen.ChatServiceClient, username string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetUserAvatar(ctx, &gen.GetUserAvatarRequest{Username: username})
	if err != nil {
		return "", err
	}
	return resp.AvatarUrl, nil
}

// loadHistory загружает историю сообщений для комнаты
func loadHistory(client gen.ChatServiceClient, roomId string, appendMsg func(string, string, string, string, string, string, []*gen.Reaction), clearMsgIDs func()) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Clear message IDs before loading new history
	clearMsgIDs()

	resp, err := client.GetHistory(ctx, &gen.GetHistoryRequest{Room: roomId})
	if err != nil {
		return err
	}

	// Collect unique users from history
	usersToLoad := make(map[string]bool)
	for _, msg := range resp.Messages {
		if msg.User != "" {
			usersToLoad[msg.User] = true
		}
	}

	// Load avatars for all users in history
	for user := range usersToLoad {
		avatarCacheMutex.Lock()
		needsLoad := avatarCache[user] == ""
		avatarCacheMutex.Unlock()

		if needsLoad {
			go func(username string) {
				avatarURL, err := getUserAvatar(client, username)
				if err == nil && avatarURL != "" {
					avatarCacheMutex.Lock()
					avatarCache[username] = avatarURL
					avatarCacheMutex.Unlock()
				}
			}(user)
		}
	}

	for _, msg := range resp.Messages {
		// Skip join messages
		if strings.HasSuffix(msg.Text, " joined") || strings.HasSuffix(msg.Text, " присоединился") {
			continue
		}
		t := msg.CreatedAt.AsTime().Local()
		timeStr := t.Format("15:04:05")
		appendMsg(timeStr, msg.User, msg.Text, msg.Id, msg.RepliedToUser, msg.RepliedToText, msg.Reactions)
	}
	return nil
}

// showChatList показывает диалог со списком чатов
func showChatList(chats []*gen.ChatInfo, switchToChat func(string)) {
	chatList := container.NewVBox()
	var chatDialog *dialog.CustomDialog

	// Check if general chat is already in the list
	hasGeneral := false
	for _, chat := range chats {
		if chat.Id == "general" {
			hasGeneral = true
			break
		}
	}

	for _, chat := range chats {
		chatName := chat.Name
		if chatName == "" {
			chatName = chat.Id
		}
		chatType := chat.Type
		if chatType == "" {
			chatType = "general"
		}

		// Create unread count badge
		var unreadBadge fyne.CanvasObject
		if chat.UnreadCount > 0 {
			badgeLabel := widget.NewLabel(fmt.Sprintf("%d", chat.UnreadCount))
			badgeLabel.TextStyle = fyne.TextStyle{Bold: true}
			unreadBadge = badgeLabel
		} else {
			unreadBadge = widget.NewLabel("")
		}

		// Create participant avatars (simplified to avoid crashes)
		var avatarsText string
		if chat.Participants != "" {
			// Try to parse as JSON array first
			participants := []string{}
			if strings.HasPrefix(chat.Participants, "[") {
				// JSON format
				cleanJSON := strings.ReplaceAll(chat.Participants, "[", "")
				cleanJSON = strings.ReplaceAll(cleanJSON, "]", "")
				cleanJSON = strings.ReplaceAll(cleanJSON, "\"", "")
				if cleanJSON != "" {
					participants = strings.Split(cleanJSON, ",")
				}
			} else {
				// Comma-separated format
				participants = strings.Split(chat.Participants, ",")
			}

			maxAvatars := 3
			for i, participant := range participants {
				if i >= maxAvatars {
					break
				}
				participant = strings.TrimSpace(participant)
				if participant != "" {
					avatarsText += "👤"
				}
			}
			if len(participants) > maxAvatars {
				avatarsText += fmt.Sprintf("+%d", len(participants)-maxAvatars)
			}
		}

		// Create display text with unread count and avatars
		displayText := fmt.Sprintf("%s %s %s", unreadBadge.(*widget.Label).Text, chatName, avatarsText)

		btn := widget.NewButton(displayText, func() {
			// Switch to selected chat
			switchToChat(chat.Id)
			// Close dialog
			if chatDialog != nil {
				chatDialog.Hide()
			}
		})
		btn.Importance = widget.LowImportance
		chatList.Add(btn)
	}

	// Add button for general chat only if it's not already in the list
	if !hasGeneral {
		generalBtn := widget.NewButton("Общий чат", func() {
			switchToChat("general")
			// Close dialog
			if chatDialog != nil {
				chatDialog.Hide()
			}
		})
		generalBtn.Importance = widget.LowImportance
		chatList.Add(generalBtn)
	}

	scroll := container.NewVScroll(chatList)
	scroll.SetMinSize(fyne.NewSize(600, 500))
	chatDialog = dialog.NewCustom("Выберите чат", "Закрыть", scroll, myWindow)
	chatDialog.Resize(fyne.NewSize(800, 600))
	chatDialog.Show()
}

// showUsersList показывает диалог со списком всех пользователей с индикаторами онлайн статуса
func showUsersList(client gen.ChatServiceClient, currentUsername string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get all users
	allUsersResp, err := client.GetAllUsers(ctx, &gen.GetAllUsersRequest{})
	if err != nil {
		dialog.ShowError(err, myWindow)
		return
	}

	// Get online users
	onlineUsersResp, err := client.GetClients(ctx, &gen.ClientListRequest{})
	if err != nil {
		dialog.ShowError(err, myWindow)
		return
	}

	// Create a set of online users for quick lookup
	onlineUsersSet := make(map[string]bool)
	for _, user := range onlineUsersResp.Clients {
		onlineUsersSet[user] = true
	}

	usersList := container.NewVBox()
	userCount := 0

	for _, userInfo := range allUsersResp.Users {
		username := userInfo.Username
		if username == currentUsername {
			continue // Skip current user
		}

		userLabel := widget.NewLabel(username)
		userLabel.TextStyle = fyne.TextStyle{Bold: true}

		// Create green circle indicator for online users
		var statusIndicator fyne.CanvasObject
		if onlineUsersSet[username] {
			statusIndicator = canvas.NewCircle(color.RGBA{R: 0, G: 255, B: 0, A: 255}) // Green
			statusIndicator.Resize(fyne.NewSize(12, 12))
		} else {
			statusIndicator = canvas.NewCircle(color.RGBA{R: 128, G: 128, B: 128, A: 255}) // Gray
			statusIndicator.Resize(fyne.NewSize(12, 12))
		}

		btn := widget.NewButton("", func() {
			// Create direct chat with selected user
			createDirectChat(client, currentUsername, username)
		})
		btn.Importance = widget.LowImportance

		userRow := container.NewHBox(statusIndicator, userLabel, btn)
		usersList.Add(userRow)
		userCount++
	}

	if userCount == 0 {
		usersList.Add(widget.NewLabel("Нет зарегистрированных пользователей"))
	}

	scroll := container.NewVScroll(usersList)
	scroll.SetMinSize(fyne.NewSize(600, 500))
	d := dialog.NewCustom("Все пользователи (кликните для создания чата)", "Закрыть", scroll, myWindow)
	d.Resize(fyne.NewSize(800, 600))
	d.Show()
}

// createDirectChat создает прямую комнату с выбранным пользователем
func createDirectChat(client gen.ChatServiceClient, user1, user2 string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.CreateDirectChat(ctx, &gen.CreateDirectChatRequest{User1: user1, User2: user2})
	if err != nil {
		dialog.ShowError(err, myWindow)
		return
	}

	if resp.Success {
		dialog.ShowInformation("Успех", fmt.Sprintf("Чат с %s создан! ID: %s", user2, resp.ChatId), myWindow)
		// Switch to the newly created chat
		switchToChat(resp.ChatId)
	} else {
		dialog.ShowError(errors.New("Не удалось создать чат"), myWindow)
	}
}

// switchToChat переключается на другой чат
func switchToChat(roomId string) {
	// Clear chat
	chatBox.Segments = []widget.RichTextSegment{}
	chatBox.Refresh()
	// Reconnect to new room
	connectToServer(roomId)
}

// checkServerAvailability проверяет доступность gRPC сервера
func checkServerAvailability(addr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("не удалось инициализировать подключение к серверу %s: %v", addr, err)
	}
	defer func() { _ = conn.Close() }()

	client := gen.NewChatServiceClient(conn)
	_, err = client.Chat(ctx)
	if err != nil && !strings.Contains(err.Error(), "stream") && !strings.Contains(err.Error(), "EOF") {
		return fmt.Errorf("сервер %s недоступен: %v", addr, err)
	}

	return nil
}

// showRegisterDialog показывает форму регистрации с выбором сервера
func showRegisterDialog(currentServer string) {
	regUsernameEntry := widget.NewEntry()
	regUsernameEntry.SetPlaceHolder("Имя пользователя")

	regPasswordEntry := widget.NewPasswordEntry()
	regPasswordEntry.SetPlaceHolder("Пароль")

	regConfirmPasswordEntry := widget.NewPasswordEntry()
	regConfirmPasswordEntry.SetPlaceHolder("Подтвердите пароль")

	regEmailEntry := widget.NewEntry()
	regEmailEntry.SetPlaceHolder("Email (необязательно)")

	// Server selector — use global serverList
	if len(serverList) == 0 {
		serverList = []ServerEntry{
			{Name: "Новый", Address: "13.140.25.249:50051"},
			{Name: "Старый", Address: "159.195.38.145:50051"},
		}
	}
	regServerNames := make([]string, len(serverList))
	regServerAddrs := make([]string, len(serverList))
	for i, s := range serverList {
		regServerNames[i] = s.Name
		regServerAddrs[i] = s.Address
	}

	serverLabel := widget.NewLabel("Сервер:")
	serverSelect := widget.NewSelect(regServerNames, func(s string) {})
	// Pre-select current server
	found := false
	for i, addr := range regServerAddrs {
		if addr == currentServer {
			serverSelect.SetSelected(regServerNames[i])
			found = true
			break
		}
	}
	if !found && len(regServerNames) > 0 {
		serverSelect.SetSelected(regServerNames[0])
	}

	registerForm := dialog.NewCustomConfirm("Регистрация", "Зарегистрироваться", "Отмена",
		container.NewVBox(
			widget.NewLabel("Имя пользователя:"),
			regUsernameEntry,
			widget.NewLabel("Пароль:"),
			regPasswordEntry,
			widget.NewLabel("Подтвердите пароль:"),
			regConfirmPasswordEntry,
			widget.NewLabel("Email:"),
			regEmailEntry,
			serverLabel,
			serverSelect,
		),
		func(b bool) {
			if b && regUsernameEntry.Text != "" {
				u := regUsernameEntry.Text
				p := regPasswordEntry.Text
				confirmP := regConfirmPasswordEntry.Text
				email := regEmailEntry.Text
				// Find address from selected name
				selectedServer := regServerAddrs[0]
				for i, name := range regServerNames {
					if name == serverSelect.Selected {
						selectedServer = regServerAddrs[i]
						break
					}
				}

				if p == "" {
					dialog.ShowError(errors.New("Введите пароль"), myWindow)
					return
				}
				if p != confirmP {
					dialog.ShowError(errors.New("Пароли не совпадают"), myWindow)
					return
				}

				// Connect to selected server
				regConn, err := grpc.NewClient(selectedServer, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					dialog.ShowError(fmt.Errorf("Не удалось подключиться к %s: %v", selectedServer, err), myWindow)
					return
				}

				regCtx, regCancel := context.WithTimeout(context.Background(), 10*time.Second)
				regClient := gen.NewChatServiceClient(regConn)
				regStream, err := regClient.Chat(regCtx)
				if err != nil {
					dialog.ShowError(fmt.Errorf("Ошибка соединения с %s: %v", selectedServer, err), myWindow)
					regConn.Close()
					regCancel()
					return
				}

				// Send registration message with Register=true
				err = regStream.Send(&gen.Message{
					User:     u,
					Password: p,
					Register: true,
					Text:     email,
				})
				if err != nil {
					dialog.ShowError(fmt.Errorf("Ошибка отправки: %v", err), myWindow)
					regConn.Close()
					regCancel()
					return
				}

				// Wait for response from server
				go func() {
					msg, err := regStream.Recv()
					if err != nil {
						fyne.Do(func() {
							dialog.ShowError(fmt.Errorf("Ошибка ответа сервера: %v", err), myWindow)
						})
						regConn.Close()
						regCancel()
						return
					}

					fyne.Do(func() {
						regCancel()
						switch msg.Text {
						case "REGISTRATION_SUCCESS":
							dialog.ShowInformation("Успех", fmt.Sprintf("Пользователь %s зарегистрирован! Входим...", u), myWindow)
							if globalCfg != nil {
								globalCfg.LastUsername = u
								globalCfg.LastPassword = p
								if email != "" {
									globalCfg.LastEmail = email
								}
								_ = saveConfig(globalCfg)
							}
							// Set credentials and auto-login
							currentUsername = u
							currentPassword = p
							regConn.Close()
							// Use the same flow as normal login
							// Connect to server
							regConn2, err := grpc.NewClient(selectedServer, grpc.WithTransportCredentials(insecure.NewCredentials()))
							if err != nil {
								dialog.ShowError(fmt.Errorf("Ошибка подключения: %v", err), myWindow)
								return
							}
							currentConn = regConn2
							client := gen.NewChatServiceClient(currentConn)

							// Load avatar
							go func() {
								avatarURL, err := getUserAvatar(client, u)
								if err == nil && avatarURL != "" {
									avatarCacheMutex.Lock()
									avatarCache[u] = avatarURL
									avatarCacheMutex.Unlock()
								}
							}()

							// Get chats and connect
							chats, err := getChats(client, u)
							if err != nil {
								connectToServer("general")
								return
							}
							if len(chats) == 0 {
								connectToServer("general")
							} else {
								showChatList(chats, connectToServer)
							}
						case "USER_ALREADY_EXISTS":
							dialog.ShowError(errors.New("Пользователь с таким именем уже существует"), myWindow)
						case "EMAIL_ALREADY_IN_USE":
							dialog.ShowError(errors.New("Этот email уже используется"), myWindow)
						default:
							dialog.ShowError(fmt.Errorf("Ошибка регистрации: %s", msg.Text), myWindow)
						}
					})
				}()
			}
		}, myWindow)

	registerForm.Show()
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		cfg = &Config{}
	}
	globalCfg = cfg

	// Читаем адрес сервера из конфига, с фоллбэком на localhost
	serverAddress := cfg.ServerAddress
	if serverAddress == "" {
		serverAddress = "13.140.25.249:50051"
	}

	// Fetch server list from GetServers RPC (async, with fallback)
	go fetchServers(serverAddress)

	isDarkTheme := cfg.CurrentTheme == "dark"
	activeThemeConfig := cfg.Themes.Light
	if isDarkTheme {
		activeThemeConfig = cfg.Themes.Dark
	}

	myTheme := &customTheme{
		isDark:       isDarkTheme,
		lightBg:      parseHexColor(cfg.Themes.Light.BgColor),
		darkBg:       parseHexColor(cfg.Themes.Dark.BgColor),
		lightFg:      parseHexColor(cfg.Themes.Light.TextColor),
		darkFg:       parseHexColor(cfg.Themes.Dark.TextColor),
		lightPrimary: parseHexColor(cfg.Themes.Light.NameColor),
		darkPrimary:  parseHexColor(cfg.Themes.Dark.NameColor),
		lightTime:    parseHexColor(cfg.Themes.Light.TimeColor),
		darkTime:     parseHexColor(cfg.Themes.Dark.TimeColor),
	}
	for _, c := range activeThemeConfig.UserColors {
		myTheme.userColors = append(myTheme.userColors, parseHexColor(c))
	}

	myApp := app.New()
	myApp.Settings().SetTheme(myTheme)

	myWindow = myApp.NewWindow(fmt.Sprintf("Lavender Messenger v%s", clientVersion))
	myWindow.Resize(fyne.NewSize(1200, 800))

	var currentRoomId string
	var stream gen.ChatService_ChatClient

	// UI для статуса (индикатор и текст)
	statusIndicator := canvas.NewCircle(color.RGBA{R: 255, G: 255, B: 0, A: 255}) // Желтый
	statusIndicator.Resize(fyne.NewSize(12, 12))

	statusLabel := widget.NewLabel("Подключение...")
	statusLabel.Alignment = fyne.TextAlignLeading

	statusBox := container.NewHBox(statusIndicator, statusLabel)

	var lastUser string
	var userColorMap = make(map[string]fyne.ThemeColorName)
	var userColorIndex int
	var messageIDs = make(map[string]bool) // Track message IDs to avoid duplicates

	getUserColorName := func(user string) fyne.ThemeColorName {
		if colorName, exists := userColorMap[user]; exists {
			return colorName
		}
		if len(myTheme.userColors) == 0 {
			return theme.ColorNamePrimary
		}
		var colorName fyne.ThemeColorName
		switch userColorIndex % 10 {
		case 0:
			colorName = UserColor1
		case 1:
			colorName = UserColor2
		case 2:
			colorName = UserColor3
		case 3:
			colorName = UserColor4
		case 4:
			colorName = UserColor5
		case 5:
			colorName = UserColor6
		case 6:
			colorName = UserColor7
		case 7:
			colorName = UserColor8
		case 8:
			colorName = UserColor9
		case 9:
			colorName = UserColor10
		}
		userColorMap[user] = colorName
		userColorIndex++
		return colorName
	}

	chatBox = widget.NewRichText()

	// Copy button for chat
	copyChatBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		// Collect all text from chat segments
		var sb strings.Builder
		for _, seg := range chatBox.Segments {
			if ts, ok := seg.(*widget.TextSegment); ok {
				sb.WriteString(ts.Text)
			}
		}
		text := sb.String()
		if text != "" {
			cb := myWindow.Clipboard()
			if cb != nil {
				cb.SetContent(text)
			}
		}
	})
	copyChatBtn.Importance = widget.LowImportance

	scrollContainer := container.NewVScroll(chatBox)

	inputBox := widget.NewEntry()
	inputBox.SetPlaceHolder("Введите сообщение...")

	var themeBtn *widget.Button
	themeBtn = widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), func() {
		myTheme.isDark = !myTheme.isDark
		if myTheme.isDark {
			themeBtn.SetIcon(theme.VisibilityOffIcon())
			cfg.CurrentTheme = "dark"
		} else {
			themeBtn.SetIcon(theme.ColorPaletteIcon())
			cfg.CurrentTheme = "light"
		}
		myApp.Settings().SetTheme(myTheme)
		chatBox.Refresh()
		_ = saveConfig(cfg)
	})
	if myTheme.isDark {
		themeBtn.SetIcon(theme.VisibilityOffIcon())
	} else {
		themeBtn.SetIcon(theme.ColorPaletteIcon())
	}

	// Chat list button
	chatListBtn := widget.NewButtonWithIcon("Чаты", theme.ListIcon(), func() {
		// Show chat list
		if currentConn != nil {
			client := gen.NewChatServiceClient(currentConn)
			chats, err := getChats(client, currentUsername)
			if err != nil {
				dialog.ShowError(err, myWindow)
				return
			}
			showChatList(chats, switchToChat)
		} else {
			dialog.ShowError(errors.New("Сначала подключитесь к серверу"), myWindow)
		}
	})

	// Users list button (all users with online status)
	usersBtn := widget.NewButtonWithIcon("Пользователи", theme.AccountIcon(), func() {
		// Show all users list with online status
		if currentConn != nil {
			client := gen.NewChatServiceClient(currentConn)
			showUsersList(client, currentUsername)
		} else {
			dialog.ShowError(errors.New("Сначала подключитесь к серверу"), myWindow)
		}
	})

	rightButtons := container.NewHBox(chatListBtn, usersBtn, copyChatBtn, themeBtn)
	topBar := container.NewBorder(nil, nil, statusBox, rightButtons)

	appendMessage := func(timeStr, user, text string, msgID string, repliedToUser, repliedToText string, reactions []*gen.Reaction) {
		// Skip all join messages (not just for current user)
		if strings.HasSuffix(text, " joined") || strings.HasSuffix(text, " присоединился") {
			return
		}

		// Skip duplicate messages by ID
		if msgID != "" {
			if messageIDs[msgID] {
				return
			}
			messageIDs[msgID] = true
		}

		isSameUser := lastUser == user
		lastUser = user
		if !isSameUser {
			chatBox.Segments = append(chatBox.Segments, &widget.TextSegment{Text: "\n", Style: widget.RichTextStyleInline})
		}

		// Show reply quote if present
		if repliedToUser != "" && repliedToText != "" {
			replyStyle := widget.RichTextStyle{ColorName: theme.ColorNameDisabled, TextStyle: fyne.TextStyle{Italic: true}}
			replySeg := &widget.TextSegment{Text: fmt.Sprintf("↳ %s: %s\n", repliedToUser, repliedToText), Style: replyStyle}
			chatBox.Segments = append(chatBox.Segments, replySeg)
		}

		// Load user avatar if not in cache
		avatarCacheMutex.Lock()
		needsLoad := avatarCache[user] == ""
		avatarCacheMutex.Unlock()

		if needsLoad && currentConn != nil {
			go func(currentUsername string) {
				client := gen.NewChatServiceClient(currentConn)
				avatarURL, err := getUserAvatar(client, currentUsername)
				if err == nil && avatarURL != "" {
					avatarCacheMutex.Lock()
					avatarCache[currentUsername] = avatarURL
					avatarCacheMutex.Unlock()
				}
			}(user)
		}

		// Always show avatar emoji
		avatarEmoji := "👤"

		avatarCacheMutex.Lock()
		hasAvatar := avatarCache[user] != ""
		avatarCacheMutex.Unlock()

		if hasAvatar {
			avatarEmoji = "👤"
		}

		headerStyle := widget.RichTextStyle{ColorName: getUserColorName(user), TextStyle: fyne.TextStyle{Bold: true}}
		headerSeg := &widget.TextSegment{Text: fmt.Sprintf("%s %s%s: ", timeStr, avatarEmoji, user), Style: headerStyle}
		textSeg := &widget.TextSegment{Text: text + "\n", Style: widget.RichTextStyleInline}
		if isSameUser {
			chatBox.Segments = append(chatBox.Segments, textSeg)
		} else {
			chatBox.Segments = append(chatBox.Segments, headerSeg, textSeg)
		}

		// Show reactions if present
		if len(reactions) > 0 {
			reactionMap := make(map[string]int)
			for _, r := range reactions {
				reactionMap[r.Emoji]++
			}
			var reactionStrs []string
			for emoji, count := range reactionMap {
				reactionStrs = append(reactionStrs, fmt.Sprintf("%s %d", emoji, count))
			}
			reactionStyle := widget.RichTextStyle{ColorName: theme.ColorNamePrimary}
			reactionSeg := &widget.TextSegment{Text: "  " + strings.Join(reactionStrs, " ") + "\n", Style: reactionStyle}
			chatBox.Segments = append(chatBox.Segments, reactionSeg)
		}

		chatBox.Refresh()
		scrollContainer.ScrollToBottom()
	}

	safeAppendSystemMessage := func(msg string) {
		fyne.Do(func() {
			seg := &widget.TextSegment{Text: msg + "\n", Style: widget.RichTextStyle{ColorName: theme.ColorNameError}}
			chatBox.Segments = append(chatBox.Segments, seg)
			chatBox.Refresh()
			scrollContainer.ScrollToBottom()
		})
	}

	sendMsg := func() {
		text := inputBox.Text
		if text == "" || stream == nil {
			return
		}
		err := stream.Send(&gen.Message{User: currentUsername, Text: text, RoomId: currentRoomId})
		if err != nil {
			safeAppendSystemMessage(fmt.Sprintf("[Ошибка отправки]: %v", err))
		}
		inputBox.SetText("")
	}

	sendBtn := widget.NewButton("Отправить", sendMsg)

	// Emoji popup
	emojis := []string{"😀", "😃", "😄", "😁", "😅", "😂", "🤣", "😊", "😇", "🙂", "😉", "😌", "😍", "🥰", "😘", "😗", "😙", "😚", "😋", "😛", "😜", "🤪", "😝", "🤗", "🤭", "🤫", "🤔", "🤐", "🤨", "😐", "😑", "😶", "😏", "😒", "🙄", "😬", "🤥", "😌", "😔", "😪", "🤤", "😴", "😷", "🤒", "🤕", "🤢", "🤮", "🤧", "🥵", "🥶", "🥴", "😵", "🤯", "🤠", "🥳", "😎", "🤓", "🧐", "😕", "😟", "🙁", "☹️", "😮", "😯", "😲", "😳", "🥺", "😦", "😧", "😨", "😰", "😥", "😢", "😭", "😱", "😖", "😣", "😞", "😓", "😩", "😫", "🥱", "😤", "😡", "😠", "🤬", "😈", "👿", "💀", "☠️", "💩", "🤡", "👹", "👺", "👻", "👽", "👾", "🤖", "❤️", "🧡", "💛", "💚", "💙", "💜", "🖤", "🤍", "🤎", "💔", "❣️", "💕", "💞", "💓", "💗", "💖", "💘", "💝", "👍", "👎", "👌", "✌️", "🤞", "🤟", "🤘", "🤙", "👈", "👉", "👆", "👇", "☝️", "✋", "🤚", "🖐", "🖖", "👋", "🤙", "💪", "🙏"}
	var emojiPopup *widget.PopUp
	emojiGrid := container.NewGridWithColumns(8)
	for _, emoji := range emojis {
		e := emoji
		emojiGrid.Add(widget.NewButton(e, func() {
			inputBox.SetText(inputBox.Text + e)
			emojiPopup.Hide()
		}))
	}
	emojiPopup = widget.NewPopUp(container.NewVScroll(emojiGrid), myWindow.Canvas())
	emojiPopup.Hide()

	emojiBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		emojiPopup.Show()
	})

	inputContainer := container.NewBorder(nil, nil, emojiBtn, sendBtn, inputBox)
	centerContent := container.NewBorder(topBar, nil, nil, nil, scrollContainer)
	mainLayout := container.NewBorder(nil, inputContainer, nil, nil, centerContent)

	myWindow.SetContent(mainLayout)
	inputBox.OnSubmitted = func(s string) { sendMsg() }

	usernameEntry := widget.NewEntry()
	usernameEntry.SetPlaceHolder("Ваше имя")
	if cfg.LastUsername != "" {
		usernameEntry.SetText(cfg.LastUsername)
	}

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Пароль")
	if cfg.LastPassword != "" {
		passwordEntry.SetText(cfg.LastPassword)
	}

	// loginForm is declared globally above, assign to it directly

	connectToServer = func(roomId string) {
		var err error
		currentConn, err = grpc.NewClient(serverAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			dialog.ShowError(err, myWindow)
			return
		}
		client := gen.NewChatServiceClient(currentConn)
		stream, err = client.Chat(context.Background())
		if err != nil {
			dialog.ShowError(err, myWindow)
			return
		}
		currentRoomId = roomId

		// Clear message IDs before loading new room
		messageIDs = make(map[string]bool)

		// Load message history
		err = loadHistory(client, roomId, appendMessage, func() {})
		if err != nil {
			safeAppendSystemMessage(fmt.Sprintf("[Ошибка загрузки истории]: %v", err))
		}

		// Send auth/join message with currentPassword and roomId
		joinMessage := fmt.Sprintf("%s присоединился", currentUsername)
		err = stream.Send(&gen.Message{User: currentUsername, Text: joinMessage, Password: currentPassword, RoomId: roomId})
		if err != nil {
			safeAppendSystemMessage(fmt.Sprintf("[Ошибка отправки]: %v", err))
		}

		statusLabel.SetText(fmt.Sprintf("Подключено к %s | %s", serverAddress, currentUsername))
		statusIndicator.FillColor = color.RGBA{R: 0, G: 255, B: 0, A: 255} // Зеленый
		statusIndicator.Refresh()

		go func() {
			defer func() {
				fyne.Do(func() {
					statusLabel.SetText("Соединение потеряно. Перезапустите клиент.")
					statusIndicator.FillColor = color.RGBA{R: 255, G: 0, B: 0, A: 255} // Красный
					statusIndicator.Refresh()
					inputBox.Disable()
					sendBtn.Disable()
					emojiBtn.Disable()
				})
			}()
			for {
				in, err := stream.Recv()
				if err != nil {
					return // Выход из горутины при любой ошибке
				}

				// Handle SYSTEM messages
				if in.User == "SYSTEM" {
					fyne.Do(func() {
						safeAppendSystemMessage(fmt.Sprintf("[SYSTEM]: %s", in.Text))
					})
					continue
				}

				// Skip join/leave messages
				if strings.HasSuffix(in.Text, " joined") || strings.HasSuffix(in.Text, " присоединился") ||
					strings.HasSuffix(in.Text, " left") || strings.HasSuffix(in.Text, " покинул") {
					continue
				}

				// Safe timestamp handling
				var timeStr string
				if in.CreatedAt != nil {
					t := in.CreatedAt.AsTime().Local()
					timeStr = t.Format("15:04:05")
				} else {
					timeStr = "00:00:00"
				}

				fyne.Do(func() {
					appendMessage(timeStr, in.User, in.Text, in.Id, in.RepliedToUser, in.RepliedToText, in.Reactions)
				})
			}
		}()
	}

	// Server selector for login — use serverList from GetServers RPC
	// Wait a moment for fetchServers to populate, fallback if empty
	time.Sleep(200 * time.Millisecond)
	if len(serverList) == 0 {
		serverList = []ServerEntry{
			{Name: "Новый", Address: "13.140.25.249:50051"},
			{Name: "Старый", Address: "159.195.38.145:50051"},
		}
	}
	serverNames := make([]string, len(serverList))
	serverAddrs := make([]string, len(serverList))
	for i, s := range serverList {
		serverNames[i] = s.Name
		serverAddrs[i] = s.Address
	}

	loginServerSelect := widget.NewSelect(serverNames, func(s string) {})
	// Pre-select current server
	loginServerFound := false
	for i, addr := range serverAddrs {
		if addr == serverAddress {
			loginServerSelect.SetSelected(serverNames[i])
			loginServerFound = true
			break
		}
	}
	if !loginServerFound && len(serverNames) > 0 {
		loginServerSelect.SetSelected(serverNames[0])
	}

	loginForm = dialog.NewCustomConfirm("Вход в чат", "Войти", "Отмена",
		container.NewVBox(
			widget.NewLabel("Сервер:"),
			loginServerSelect,
			widget.NewLabel("Имя пользователя:"),
			usernameEntry,
			widget.NewLabel("Пароль:"),
			passwordEntry,
			widget.NewLabel(""),
			widget.NewButton("Нет аккаунта? Зарегистрироваться", func() {
				// Find address from selected name
				selectedAddr := serverAddress
				for i, name := range serverNames {
					if name == loginServerSelect.Selected {
						selectedAddr = serverAddrs[i]
						break
					}
				}
				loginForm.Hide()
				showRegisterDialog(selectedAddr)
			}),
		),
		func(b bool) {
			if b && usernameEntry.Text != "" {
				loginForm.Hide()
				currentUsername = usernameEntry.Text
				currentPassword = passwordEntry.Text
				// Update server address from selector
				for i, name := range serverNames {
					if name == loginServerSelect.Selected {
						serverAddress = serverAddrs[i]
						break
					}
				}
				cfg.LastUsername = currentUsername
				cfg.LastPassword = currentPassword
				_ = saveConfig(cfg)

				// Connect to server first
				var err error
				currentConn, err = grpc.NewClient(serverAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					dialog.ShowError(err, myWindow)
					myApp.Quit()
					return
				}
				client := gen.NewChatServiceClient(currentConn)

				// Load current user avatar
				go func() {
					avatarURL, err := getUserAvatar(client, currentUsername)
					if err == nil && avatarURL != "" {
						avatarCacheMutex.Lock()
						avatarCache[currentUsername] = avatarURL
						avatarCacheMutex.Unlock()
					}
				}()

				// Get chats
				chats, err := getChats(client, currentUsername)
				if err != nil {
					dialog.ShowError(fmt.Errorf("Ошибка получения чатов: %v", err), myWindow)
					connectToServer("general")
					return
				}

				if len(chats) == 0 {
					connectToServer("general")
				} else {
					showChatList(chats, connectToServer)
				}
			} else {
				myApp.Quit()
			}
		}, myWindow)

	myWindow.Show()

	// Асинхронно проверяем доступность сервера после отображения окна
	go func() {
		err := checkServerAvailability(serverAddress)
		fyne.Do(func() {
			if err != nil {
				statusLabel.SetText("Сервер недоступен")
				statusIndicator.FillColor = color.RGBA{R: 255, G: 0, B: 0, A: 255} // Красный
				statusIndicator.Refresh()
				inputBox.Disable()
				sendBtn.Disable()
				emojiBtn.Disable()
			} else {
				statusLabel.SetText("Ожидание входа...")
				statusIndicator.FillColor = color.RGBA{R: 255, G: 255, B: 0, A: 255} // Желтый
				statusIndicator.Refresh()
			}
			// Always show login form so user can try to connect or register
			loginForm.Show()
		})
	}()

	myApp.Run()

	if currentConn != nil {
		_ = currentConn.Close()
	}
}
