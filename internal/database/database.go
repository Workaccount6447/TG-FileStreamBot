package database

import (
	"context"
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
	ID       int64     `bson:"id"`
	JoinDate time.Time `bson:"join_date"`
	Links    int64     `bson:"links"`
}

type BannedUser struct {
	ID      int64     `bson:"id"`
	BanDate time.Time `bson:"ban_date"`
}

type DB struct {
	client  *mongo.Client
	users   *mongo.Collection
	banned  *mongo.Collection
	links   *mongo.Collection
}

var instance *DB

func Init(uri string) error {
	if uri == "" {
		return nil // database is optional
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return err
	}
	db := client.Database("fsb")
	instance = &DB{
		client:  client,
		users:   db.Collection("users"),
		banned:  db.Collection("blacklist"),
		links:   db.Collection("links"),
	}
	return nil
}

func IsEnabled() bool {
	return instance != nil
}

func GetDB() *DB {
	return instance
}

// ---- User Management ----

func (d *DB) AddUser(ctx context.Context, id int64) error {
	user := User{ID: id, JoinDate: time.Now(), Links: 0}
	_, err := d.users.InsertOne(ctx, user)
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

// ---- Link Stats ----

func (d *DB) TotalLinks(ctx context.Context) (int64, error) {
	return d.links.CountDocuments(ctx, bson.M{})
}

func (d *DB) AddLink(ctx context.Context, userID int64, link string) error {
	_, err := d.links.InsertOne(ctx, bson.M{
		"user_id":    userID,
		"link":       link,
		"created_at": time.Now(),
	})
	return err
}
