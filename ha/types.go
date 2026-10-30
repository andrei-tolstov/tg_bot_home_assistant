package ha

// ControllableDomains - список доменов Home Assistant, которыми можно управлять через бота.
var ControllableDomains = []string{
	"light",
	"switch",
	"fan",
	"humidifier",
	"cover",
	"climate",
	"lock",
	"media_player",
	"vacuum",
	"water_heater",
	"valve",
	"siren",
	"input_boolean",
	"automation",
	"scene",
	"script",
}

// DomainPriority определяет порядок отображения категорий в списках и клавиатурах.
var DomainPriority = map[string]int{
	"light":         1,
	"switch":        2,
	"fan":           3,
	"humidifier":    4,
	"climate":       5,
	"cover":         6,
	"water_heater":  7,
	"valve":         8,
	"siren":         9,
	"lock":          10,
	"media_player":  11,
	"vacuum":        12,
	"input_boolean": 13,
	"automation":    14,
	"scene":         15,
	"script":        16,
	"number":        17,
	"input_number":  17,
	"select":        18,
	"input_select":  18,
	"button":        19,
	"input_button":  19,
	"time":          20,
}

// GetDomainPriority возвращает числовой приоритет сортировки для домена (меньше - выше).
func GetDomainPriority(domain string) int {
	if p, ok := DomainPriority[domain]; ok {
		return p
	}
	return 50
}

// Entity представляет сущность (устройство, настройку, датчик или автоматизацию) в Home Assistant.
type Entity struct {
	EntityID     string                 `json:"entity_id"`
	Domain       string                 `json:"domain"`
	FriendlyName string                 `json:"friendly_name"`
	State        string                 `json:"state"`
	Category     string                 `json:"category,omitempty"`      // "config", "diagnostic" или "" (основное)
	IsHidden     bool                   `json:"is_hidden,omitempty"`     // true, если скрыто
	DeviceID     string                 `json:"device_id,omitempty"`     // ID физического устройства в HA
	DeviceName   string                 `json:"device_name,omitempty"`   // Название физического устройства
	Attributes   map[string]interface{} `json:"attributes,omitempty"`    // Атрибуты (min, max, step, options, unit_of_measurement и др.)
}

// DeviceConfigGroup группирует конфигурационные сущности одного физического устройства.
type DeviceConfigGroup struct {
	DeviceID       string   `json:"device_id"`
	DeviceName     string   `json:"device_name"`
	Domain         string   `json:"domain"`
	PrimaryEntity  *Entity  `json:"primary_entity,omitempty"`
	ConfigEntities []Entity `json:"config_entities"`
}

// EntityState представляет сырое состояние сущности, возвращаемое API Home Assistant.
type EntityState struct {
	EntityID   string                 `json:"entity_id"`
	State      string                 `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

// APIStatusResponse представляет ответ от эндпоинта /api/.
type APIStatusResponse struct {
	Message string `json:"message"`
}

