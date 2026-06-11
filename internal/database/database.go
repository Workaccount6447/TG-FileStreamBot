package database

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrUserNotFound = errors.New("user not found")
var ErrUserAlreadyBanned = errors.New("user already banned")
var ErrUserNotBanned = errors.New("user not banned")

type User struct {
	ID    int64 `bson:"id"`
	Links int64 `bson:"links"`
}

type BannedUser struct {
	ID      int64     `bson:"id"`
	BanDate time.Time `bson:"ban_date"`
}

// FileLink now stores only what cannot be fetched live from the log channel.
// user_id   — who owns the file
// message_id — message ID in the log channel (used to fetch file info live)
// created_at — for sorting newest-first in /myfiles
type FileLink struct {
	UserID    int64     `bson:"user_id"`
	MessageID int       `bson:"message_id"`
	CreatedAt time.Time `bson:"created_at"`
}

type DB struct {
	client *mongo.Client
	users  *mongo.Collection
	banned *mongo.Collection
	links  *mongo.Collection
}

var instance *DB

func Init(uri string) error {
	if uri == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)

	if clientOpts.TLSConfig == nil {
		clientOpts.SetTLSConfig(&tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		})
	}

	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		clientOpts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec
		client, err = mongo.Connect(ctx, clientOpts)
		if err != nil {
			return err
		}
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer pingCancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		clientOpts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec
		client2, err2 := mongo.Connect(context.Background(), clientOpts)
		if err2 != nil {
			return err
		}
		pingCtx2, pingCancel2 := context.WithTimeout(context.Background(), 15*time.Second)
		defer pingCancel2()
		if err2 = client2.Ping(pingCtx2, nil); err2 != nil {
			return err
		}
		client = client2
	}

	db := client.Database("fsb")
	instance = &DB{
		client: client,
		users:  db.Collection("users"),
		banned: db.Collection("blacklist"),
		links:  db.Collection("links"),
	}

	// Index for fast per-user pagination newest-first
	instance.links.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}},
		Options: options.Index().SetBackground(true),
	})

	instance.MigrateDropJoinDate(context.Background())

	return nil
}

func IsEnabled() bool { return instance != nil }
func GetDB() *DB      { return instance }

// ---- User Management ----

func (d *DB) AddUser(ctx context.Context, id int64) error {
	_, err := d.users.InsertOne(ctx, User{ID: id, Links: 0})
	return err
}

func (d *DB) GetUser(ctx context.Context, id int64) (*User, error) {
	var user User
	err := d.users.FindOne(ctx, bson.M{"id": id}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrUserNotFound
	}
	return &user, err
}

func (d *DB) UserExists(ctx context.Context, id int64) bool {
	_, err := d.GetUser(ctx, id)
	return err == nil
}

func (d *DB) DeleteUser(ctx context.Context, id int64) error {
	_, err := d.users.DeleteMany(ctx, bson.M{"id": id})
	return err
}

func (d *DB) TotalUsers(ctx context.Context) (int64, error) {
	return d.users.CountDocuments(ctx, bson.M{})
}

func (d *DB) GetAllUsers(ctx context.Context) (*mongo.Cursor, error) {
	return d.users.Find(ctx, bson.M{})
}

func (d *DB) IncrLinks(ctx context.Context, id int64) {
	d.users.UpdateOne(ctx, bson.M{"id": id}, bson.M{"$inc": bson.M{"links": 1}})
}

// ---- Ban Management ----

func (d *DB) BanUser(ctx context.Context, id int64) error {
	if d.IsUserBanned(ctx, id) {
		return ErrUserAlreadyBanned
	}
	_, err := d.banned.InsertOne(ctx, BannedUser{ID: id, BanDate: time.Now()})
	return err
}

func (d *DB) UnbanUser(ctx context.Context, id int64) error {
	if !d.IsUserBanned(ctx, id) {
		return ErrUserNotBanned
	}
	_, err := d.banned.DeleteOne(ctx, bson.M{"id": id})
	return err
}

func (d *DB) IsUserBanned(ctx context.Context, id int64) bool {
	count, _ := d.banned.CountDocuments(ctx, bson.M{"id": id})
	return count > 0
}

func (d *DB) TotalBanned(ctx context.Context) (int64, error) {
	return d.banned.CountDocuments(ctx, bson.M{})
}

// ---- Link Management ----

func (d *DB) TotalLinks(ctx context.Context) (int64, error) {
	return d.links.CountDocuments(ctx, bson.M{})
}

// AddFileLink stores only user_id, message_id, created_at.
func (d *DB) AddFileLink(ctx context.Context, userID int64, messageID int) error {
	_, err := d.links.InsertOne(ctx, FileLink{
		UserID:    userID,
		MessageID: messageID,
		CreatedAt: time.Now(),
	})
	return err
}

// GetUserFiles returns one page of FileLink records for a user, newest first.
func (d *DB) GetUserFiles(ctx context.Context, userID int64, page, perPage int) ([]FileLink, int64, error) {
	filter := bson.M{"user_id": userID}
	total, err := d.links.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64(page * perPage)).
		SetLimit(int64(perPage))

	cursor, err := d.links.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var files []FileLink
	if err := cursor.All(ctx, &files); err != nil {
		return nil, 0, err
	}
	return files, total, nil
}

// DeleteUserFiles removes ALL file links for a user. Used by /clearfiles.
func (d *DB) DeleteUserFiles(ctx context.Context, userID int64) (int64, error) {
	res, err := d.links.DeleteMany(ctx, bson.M{"user_id": userID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// GetUserStats returns personal stats. Size is not stored anymore — only file count and link count.
func (d *DB) GetUserStats(ctx context.Context, userID int64) (*UserStats, error) {
	filter := bson.M{"user_id": userID}

	totalFiles, err := d.links.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	opts := options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}})
	var newest FileLink
	var newestPtr *FileLink
	if err := d.links.FindOne(ctx, filter, opts).Decode(&newest); err == nil {
		newestPtr = &newest
	}

	user, err := d.GetUser(ctx, userID)
	var linksCount int64
	if err == nil {
		linksCount = user.Links
	}

	return &UserStats{
		TotalFiles: totalFiles,
		LinksCount: linksCount,
		NewestFile: newestPtr,
	}, nil
}

// UserStats — TotalSize removed since we no longer store file_size in DB.
type UserStats struct {
	TotalFiles int64
	LinksCount int64
	NewestFile *FileLink
}

func (d *DB) MigrateDropJoinDate(ctx context.Context) {
	d.users.UpdateMany(
		ctx,
		bson.M{"join_date": bson.M{"$exists": true}},
		bson.M{"$unset": bson.M{"join_date": ""}},
	)
}
