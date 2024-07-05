package preferences

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"

	"github.com/jmoiron/sqlx"
	"go.signoz.io/signoz/pkg/query-service/common"
	"go.signoz.io/signoz/pkg/query-service/model"
)

var db *sqlx.DB

type Preference struct {
	Id           string `json:"id" db:"id"`
	Name         string `json:"name" db:"name"`
	DefaultValue string `json:"default_value" db:"default_value"`
	DependsOn    string `json:"depends_on" db:"depends_on"`
	UserScope    int    `json:"user" db:"user"`
	OrgScope     int    `json:"org" db:"org"`
	GroupId      string `json:"group_id" db:"group_id"`
}

type PreferenceGroup struct {
	Id          string `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	ParentGroup string `json:"parent_group" db:"parent_group"`
}

type PreferenceKV struct {
	PreferenceId    string `json:"preference_id" db:"preference_id"`
	PreferenceValue string `json:"preference_value" db:"preference_value"`
}

type UpdateOrgPreferenceRequest struct {
	PreferenceId    string `json:"preference_id"`
	PreferenceValue string `json:"preference_value"`
}

type AllPreferenceResponse struct {
	GroupId     string                  `json:"group_id"`
	GroupName   string                  `json:"group_name"`
	Preferences []Preferences           `json:"preferences"`
	ChildGroup  []AllPreferenceResponse `json:"child_groups"`
}

type Preferences struct {
	Preference
	Value string `json:"value" db:"value"`
}

func InitDB(datasourceName string) error {
	var err error

	db, err = sqlx.Open("sqlite3", datasourceName)

	if err != nil {
		return err
	}

	// create the preference group entity
	table_schema := `CREATE TABLE IF NOT EXISTS preference_group(
		id TEXT PRIMARY KEY NOT NULL,
		name TEXT,
		parent_group TEXT
	);`

	_, err = db.Exec(table_schema)
	if err != nil {
		return fmt.Errorf("error in creating preference group table: %s", err.Error())
	}
	// bootstrap the preference group entity data
	bootstrapPreferenceGroup, fileError := fs.ReadFile(os.DirFS("../../pkg/query-service/app/preferences"), "bootstrap_preference_groups.json")

	if fileError != nil {
		return fmt.Errorf("error in reading bootstrap preference group: %s", fileError.Error())
	}

	preferenceGroups := []PreferenceGroup{}

	if unmarshalErr := json.Unmarshal(bootstrapPreferenceGroup, &preferenceGroups); unmarshalErr != nil {
		return fmt.Errorf("error in unmarshalling bootstrap preference groups: %s", unmarshalErr.Error())
	}

	for _, preferenceGroup := range preferenceGroups {

		var preferenceGroupFromDB PreferenceGroup
		query := `SELECT id FROM preference_group WHERE id=$1;`
		err = db.Get(&preferenceGroupFromDB, query, preferenceGroup.Id)

		// if the preference group is not present then insert the same
		if err == sql.ErrNoRows {
			query = `INSERT INTO preference_group(id,name,parent_group) VALUES($1,$2,$3);`

			_, err = db.Exec(query, preferenceGroup.Id, preferenceGroup.Name, preferenceGroup.ParentGroup)

			if err != nil {
				return fmt.Errorf("error in adding bootstrap preference group: %s", err.Error())
			}
		} else if err != nil {
			// if there is some other error in query return from here
			return fmt.Errorf("error in bootstrap entries in preference group entity: %s", err.Error())
		}

	}

	// create the preference entity
	table_schema = `CREATE TABLE IF NOT EXISTS preference(
		id TEXT PRIMARY KEY NOT NULL,
		name TEXT,
		default_value TEXT,
		depends_on TEXT,
		user INTEGER DEFAULT 0,
		org INTEGER DEFAULT 0,
		group_id TEXT,
		FOREIGN KEY (group_id)
        	REFERENCES preference_group(id)
        	ON UPDATE CASCADE
        	ON DELETE CASCADE
	);`

	_, err = db.Exec(table_schema)
	if err != nil {
		return fmt.Errorf("error in creating preference table: %s", err.Error())
	}

	// bootstrap the preference entity data
	bootstrapPreferences, fileError := fs.ReadFile(os.DirFS("../../pkg/query-service/app/preferences"), "bootstrap_preferences.json")

	if fileError != nil {
		return fmt.Errorf("error in reading bootstrap preferences: %s", fileError.Error())
	}

	preferences := []Preference{}

	if unmarshalErr := json.Unmarshal(bootstrapPreferences, &preferences); unmarshalErr != nil {
		return fmt.Errorf("error in unmarshalling bootstrap preferences: %s", unmarshalErr.Error())
	}

	for _, preference := range preferences {

		var preferenceFromDB Preference
		query := `SELECT id FROM preference WHERE id=$1;`
		err = db.Get(&preferenceFromDB, query, preference.Id)

		// if the preference entity is not present insert the same
		if err == sql.ErrNoRows {
			query = `INSERT INTO preference(id,name,default_value,depends_on,user,org,group_id) VALUES($1,$2,$3,$4,$5,$6,$7);`

			_, err = db.Exec(query, preference.Id, preference.Name, preference.DefaultValue, preference.DependsOn, preference.UserScope, preference.OrgScope, preference.GroupId)

			if err != nil {
				return fmt.Errorf("error in adding bootstrap preference: %s", err.Error())
			}
		} else if err != nil {
			// if there is some other error in query return from here
			return fmt.Errorf("error in bootstrap entries in preference entity: %s", err.Error())
		}

	}

	// create the user preference table
	table_schema = `
	PRAGMA foreign_keys = ON;
	CREATE TABLE IF NOT EXISTS user_preference(
		preference_id TEXT NOT NULL,
		preference_value TEXT,
		user_id TEXT NOT NULL,
		PRIMARY KEY (preference_id,user_id),
		FOREIGN KEY (preference_id)
			REFERENCES preference(id)
			ON UPDATE CASCADE 
			ON DELETE CASCADE,
		FOREIGN KEY (user_id)
			REFERENCES users(id)
			ON UPDATE CASCADE
			ON DELETE CASCADE
	);`

	_, err = db.Exec(table_schema)
	if err != nil {
		return fmt.Errorf("error in creating user_preference table: %s", err.Error())
	}

	// create the org preference table
	table_schema = `
	PRAGMA foreign_keys = ON;
	CREATE TABLE IF NOT EXISTS org_preference(
		preference_id TEXT NOT NULL,
		preference_value TEXT,
		org_id TEXT NOT NULL,
		PRIMARY KEY (preference_id,org_id),
		FOREIGN KEY (preference_id)
			REFERENCES preference(id)
			ON UPDATE CASCADE 
			ON DELETE CASCADE,
		FOREIGN KEY (org_id)
			REFERENCES organizations(id)
			ON UPDATE CASCADE
			ON DELETE CASCADE
	);`

	_, err = db.Exec(table_schema)
	if err != nil {
		return fmt.Errorf("error in creating org_preference table: %s", err.Error())
	}

	// if there is no error then return nil
	return nil
}

func GetUserPreference(ctx context.Context, preferenceId string) (*PreferenceKV, *model.ApiError) {
	userPreference := PreferenceKV{}
	orgPreference := PreferenceKV{}
	user := common.GetUserFromContext(ctx)
	orgId := user.OrgId

	// get the preference id and value from the user preference table
	query := `SELECT preference_id, preference_value FROM user_preference WHERE preference_id = $1 AND user_id = $2;`
	err := db.Get(&userPreference, query, preferenceId, user.Id)

	if err != nil && err != sql.ErrNoRows {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: err}
	}

	// get the preference id and value from the org preference table
	query = `SELECT preference_id,preference_value FROM org_preference WHERE preference_id=$1 AND org_id=$2;`
	err = db.Get(&orgPreference, query, preferenceId, orgId)

	if err != nil && err != sql.ErrNoRows {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: err}
	}

	// get the details for the preference entity
	preference := Preference{}
	query = `SELECT user, org , default_value FROM preference WHERE id = $1;`
	err = db.Get(&preference, query, preferenceId)

	// return if unable to fetch the preference entity as we won't be sure about preference being enabled or not also if the preference doesn't exist

	if err == sql.ErrNoRows {
		return nil, &model.ApiError{Typ: model.ErrorNotFound, Err: fmt.Errorf("no preference found with preference Id: %s", preferenceId)}
	} else if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error while fetching the preference entity: %s", err)}
	}

	// return err if the preference is not enabled for user scope
	if preference.UserScope != 1 {
		return nil, &model.ApiError{Typ: model.ErrorForbidden, Err: fmt.Errorf("preference not enabled for user scope with key: %s", preferenceId)}
	}

	preferenceValue := PreferenceKV{PreferenceId: preferenceId, PreferenceValue: preference.DefaultValue}

	if preference.OrgScope == 1 && orgPreference.PreferenceId != "" {
		preferenceValue.PreferenceValue = orgPreference.PreferenceValue
	}

	if userPreference.PreferenceId != "" {
		preferenceValue.PreferenceValue = userPreference.PreferenceValue
	}

	return &preferenceValue, nil
}

func UpdateUserPreference(ctx context.Context, req *PreferenceKV) (*PreferenceKV, *model.ApiError) {
	preferenceId := req.PreferenceId
	preferenceValue := req.PreferenceValue
	user := common.GetUserFromContext(ctx)

	// return error if there is no preference id in the request
	if preferenceId == "" {
		return nil, &model.ApiError{Typ: model.ErrorNotFound, Err: fmt.Errorf("no preference id found in the request")}
	}

	preference := Preference{}
	query := `SELECT user FROM preference WHERE id=$1;`
	err := db.Get(&preference, query, preferenceId)

	if err == sql.ErrNoRows {
		return nil, &model.ApiError{Typ: model.ErrorNotFound, Err: fmt.Errorf("no such preference exists: %s", preferenceId)}
	} else if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in fetching the preference: %s", err)}
	}

	if preference.UserScope != 1 {
		return nil, &model.ApiError{Typ: model.ErrorForbidden, Err: fmt.Errorf("this preference is not enabled at user scope: %s", preferenceId)}
	}

	query = `INSERT INTO user_preference(preference_id,preference_value,user_id) VALUES($1,$2,$3)
	ON CONFLICT(preference_id,user_id) DO
	UPDATE SET preference_value= $2 WHERE preference_id=$1 AND user_id=$3;
	`

	_, err = db.Exec(query, preferenceId, preferenceValue, user.Id)

	if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in setting the preference value: %s", err)}
	}

	return &PreferenceKV{
		PreferenceId:    preferenceId,
		PreferenceValue: preferenceValue,
	}, nil
}

func GetOrgPreference(ctx context.Context, preferenceId string) (*PreferenceKV, *model.ApiError) {
	orgPreference := PreferenceKV{}
	orgId := common.GetUserFromContext(ctx).OrgId

	// get the preference id and value from the org preference table
	query := `SELECT preference_id, preference_value FROM org_preference WHERE preference_id = $1 AND org_id = $2;`
	err := db.Get(&orgPreference, query, preferenceId, orgId)

	if err != nil && err != sql.ErrNoRows {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: err}
	}

	// get the details for the preference entity
	preference := Preference{}
	query = `SELECT org, default_value FROM preference WHERE id = $1;`
	err = db.Get(&preference, query, preferenceId)

	// return if unable to fetch the preference entity as we won't be sure about preference being enabled or not
	if err == sql.ErrNoRows {
		return nil, &model.ApiError{Typ: model.ErrorNotFound, Err: fmt.Errorf("no such preference exists: %s", preferenceId)}
	} else if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in fetching the preference: %s", err)}
	}

	// return err if the preference is not enabled for org scope
	if preference.OrgScope != 1 {
		return nil, &model.ApiError{Typ: model.ErrorForbidden, Err: fmt.Errorf("preference not enabled with key: %s", preferenceId)}
	}

	preferenceValue := PreferenceKV{PreferenceId: preferenceId, PreferenceValue: preference.DefaultValue}

	if orgPreference.PreferenceId != "" {
		preferenceValue.PreferenceValue = orgPreference.PreferenceValue
	}

	return &preferenceValue, nil
}

func UpdateOrgPreference(ctx context.Context, req *UpdateOrgPreferenceRequest) (*PreferenceKV, *model.ApiError) {
	preferenceId := req.PreferenceId
	preferenceValue := req.PreferenceValue
	orgId := common.GetUserFromContext(ctx).OrgId

	// return error if there is no preference id in the request
	if preferenceId == "" {
		return nil, &model.ApiError{Typ: model.ErrorNotFound, Err: fmt.Errorf("no preference id found in the request")}
	}

	if orgId == "" {
		return nil, &model.ApiError{Typ: model.ErrorNotFound, Err: fmt.Errorf("no org id found in the request")}
	}

	preference := Preference{}
	query := `SELECT org FROM preference WHERE id=$1;`
	err := db.Get(&preference, query, preferenceId)

	if err == sql.ErrNoRows {
		return nil, &model.ApiError{Typ: model.ErrorNotFound, Err: fmt.Errorf("no such preference exists: %s", preferenceId)}
	} else if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in fetching the preference: %s", err)}
	}

	if preference.OrgScope != 1 {
		return nil, &model.ApiError{Typ: model.ErrorForbidden, Err: fmt.Errorf("this preference is not enabled at org scope: %s", preferenceId)}
	}

	query = `INSERT INTO org_preference(preference_id,preference_value,org_id) VALUES($1,$2,$3)
	ON CONFLICT(preference_id,org_id) DO
	UPDATE SET preference_value= $2 WHERE preference_id=$1 AND org_id=$3;
	`

	_, err = db.Exec(query, preferenceId, preferenceValue, orgId)

	if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in setting the preference value: %s", err)}
	}

	return &PreferenceKV{
		PreferenceId:    preferenceId,
		PreferenceValue: preferenceValue,
	}, nil
}

func GetAllOrgPreferences(ctx context.Context) (*[]AllPreferenceResponse, *model.ApiError) {

	orgId := common.GetUserFromContext(ctx).OrgId
	orgPreferencesWithGroups := []Preference{}
	query := `
	SELECT 
        id,
        name,
        default_value,
	    depends_on,
	    user,
	    org,
        group_id
  	FROM 
        preference
  	WHERE preference.org = 1;`

	err := db.Select(&orgPreferencesWithGroups, query, orgId)

	if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in getting all org preferences: %s", err)}
	}

	if len(orgPreferencesWithGroups) == 0 {
		return nil, nil
	}

	orgPreferenceValues := []PreferenceKV{}
	query = `
	SELECT preference_id, preference_value FROM org_preference WHERE org_id=$1;
	`
	err = db.Select(&orgPreferenceValues, query, orgId)

	if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in getting all org preference values: %s", err)}
	}

	preferenceValueMap := map[string]string{}

	for _, preferenceValue := range orgPreferenceValues {
		preferenceValueMap[preferenceValue.PreferenceId] = preferenceValue.PreferenceValue
	}

	groupPreferenceMap := map[string][]Preferences{}

	for _, preference := range orgPreferencesWithGroups {
		preferenceWithValue := Preferences{}
		if preferenceValueMap[preference.Id] == "" {
			preferenceWithValue.Value = preference.DefaultValue
		} else {
			preferenceWithValue.Value = preferenceValueMap[preference.Id]
		}
		preferenceWithValue.Id = preference.Id
		preferenceWithValue.Name = preference.Name
		preferenceWithValue.DefaultValue = preference.DefaultValue
		preferenceWithValue.GroupId = preference.GroupId
		preferenceWithValue.DependsOn = preference.DependsOn
		preferenceWithValue.UserScope = preference.UserScope
		preferenceWithValue.OrgScope = preference.OrgScope

		value, seen := groupPreferenceMap[preference.GroupId]
		if !seen {
			groupPreferenceMap[preference.GroupId] = append([]Preferences{}, preferenceWithValue)
		} else {
			groupPreferenceMap[preference.GroupId] = append(value, preferenceWithValue)
		}
	}

	preferenceGroups := []PreferenceGroup{}

	query = `SELECT * FROM preference_group;`

	err = db.Select(&preferenceGroups, query)

	if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in getting all preference groups: %s", err)}
	}

	allOrgPreferenceTree := buildGroupTree(preferenceGroups, "", groupPreferenceMap)

	return &allOrgPreferenceTree, nil
}

func GetAllUserPreferences(ctx context.Context) (*[]AllPreferenceResponse, *model.ApiError) {

	user := common.GetUserFromContext(ctx)
	orgId := user.OrgId

	allUserPreferencesWithGroups := []Preference{}
	query := `
	SELECT 
        id,
        name,
        default_value,
	    depends_on,
	    user,
	    org,
        group_id
  	FROM 
        preference
  	WHERE preference.user = 1;`

	err := db.Select(&allUserPreferencesWithGroups, query, user.Id, orgId)

	if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in getting all user preferences: %s", err)}
	}

	if len(allUserPreferencesWithGroups) == 0 {
		return nil, nil
	}

	orgPreferenceValues := []PreferenceKV{}
	query = `
	SELECT preference_id, preference_value FROM org_preference WHERE org_id=$1;
	`
	err = db.Select(&orgPreferenceValues, query, orgId)

	if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in getting all org preference values: %s", err)}
	}

	orgPreferenceValueMap := map[string]string{}

	for _, preferenceValue := range orgPreferenceValues {
		orgPreferenceValueMap[preferenceValue.PreferenceId] = preferenceValue.PreferenceValue
	}

	userPreferenceValues := []PreferenceKV{}
	query = `
	SELECT preference_id, preference_value FROM user_preference WHERE user_id=$1;
	`
	err = db.Select(&userPreferenceValues, query, user.Id)

	if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in getting all user preference values: %s", err)}
	}

	userPreferenceValueMap := map[string]string{}

	for _, preferenceValue := range userPreferenceValues {
		userPreferenceValueMap[preferenceValue.PreferenceId] = preferenceValue.PreferenceValue
	}

	groupPreferenceMap := map[string][]Preferences{}

	for _, preference := range allUserPreferencesWithGroups {
		preferenceWithValue := Preferences{}
		if preference.OrgScope == 1 {
			preferenceWithValue.Value = orgPreferenceValueMap[preference.Id]
		}

		if preference.UserScope == 1 {
			if value, seen := userPreferenceValueMap[preference.Id]; seen {
				preferenceWithValue.Value = value
			}
		}
		if preferenceWithValue.Value == "" {
			preferenceWithValue.Value = preference.DefaultValue
		}
		preferenceWithValue.Id = preference.Id
		preferenceWithValue.Name = preference.Name
		preferenceWithValue.DefaultValue = preference.DefaultValue
		preferenceWithValue.GroupId = preference.GroupId
		preferenceWithValue.DependsOn = preference.DependsOn
		preferenceWithValue.UserScope = preference.UserScope
		preferenceWithValue.OrgScope = preference.OrgScope

		value, seen := groupPreferenceMap[preference.GroupId]
		if !seen {
			groupPreferenceMap[preference.GroupId] = append([]Preferences{}, preferenceWithValue)
		} else {
			groupPreferenceMap[preference.GroupId] = append(value, preferenceWithValue)
		}
	}

	preferenceGroups := []PreferenceGroup{}

	query = `SELECT * FROM preference_group;`

	err = db.Select(&preferenceGroups, query)

	if err != nil {
		return nil, &model.ApiError{Typ: model.ErrorExec, Err: fmt.Errorf("error in getting all preference groups: %s", err)}
	}

	allUserPreferenceTree := buildGroupTree(preferenceGroups, "", groupPreferenceMap)

	return &allUserPreferenceTree, nil
}

// recursively create the preference group tree
func buildGroupTree(groups []PreferenceGroup, parentID string, groupPreferenceMap map[string][]Preferences) []AllPreferenceResponse {
	tree := []AllPreferenceResponse{}

	for _, group := range groups {
		treeNode := AllPreferenceResponse{}
		if group.ParentGroup == parentID {
			children := buildGroupTree(groups, group.Id, groupPreferenceMap)
			treeNode.GroupId = group.Id
			treeNode.GroupName = group.Name
			treeNode.ChildGroup = children
			if val, seen := groupPreferenceMap[group.Id]; seen {
				treeNode.Preferences = val
			}
			// if there are no preferences in the current group and there are no child groups then do not push the node to the tree
			if len(children) == 0 && treeNode.Preferences == nil {
				continue
			} else {
				tree = append(tree, treeNode)
			}
		}
	}

	return tree
}
