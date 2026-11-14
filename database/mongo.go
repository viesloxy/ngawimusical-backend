package database

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/viesloxy/ngawimusical-backend/config"
)

// NewMongo membuat koneksi MongoDB untuk modul ulasan.
func NewMongo(ctx context.Context, cfg config.MongoConfig) (*mongo.Client, error) {
	opts := options.Client().
		ApplyURI(cfg.URI).
		SetConnectTimeout(cfg.ConnectTimeout).
		SetServerSelectionTimeout(cfg.ConnectTimeout)

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("koneksi mongodb: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}

	return client, nil
}

// EnsureReviewIndexes membuat index yang dibutuhkan modul ulasan.
func EnsureReviewIndexes(ctx context.Context, db *mongo.Database) error {
	type collectionIndex struct {
		collection string
		indexes    []mongo.IndexModel
	}

	collections := []collectionIndex{
		{
			collection: "reviews",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "event_id", Value: 1}, {Key: "user_id", Value: 1}},
					Options: options.Index().SetName("reviews_event_user_unique").SetUnique(true),
				},
				{
					Keys:    bson.D{{Key: "event_id", Value: 1}, {Key: "created_at", Value: -1}, {Key: "_id", Value: -1}},
					Options: options.Index().SetName("reviews_event_created"),
				},
				{
					Keys:    bson.D{{Key: "tags", Value: 1}},
					Options: options.Index().SetName("reviews_tags"),
				},
			},
		},
		{
			collection: "review_replies",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "review_id", Value: 1}, {Key: "created_at", Value: 1}, {Key: "_id", Value: 1}},
					Options: options.Index().SetName("review_replies_review_cursor"),
				},
			},
		},
		{
			collection: "helpful_votes",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "review_id", Value: 1}, {Key: "user_id", Value: 1}},
					Options: options.Index().SetName("helpful_votes_review_user_unique").SetUnique(true),
				},
			},
		},
	}

	for _, item := range collections {
		if _, err := db.Collection(item.collection).Indexes().CreateMany(ctx, item.indexes); err != nil {
			return fmt.Errorf("buat index koleksi %s: %w", item.collection, err)
		}
	}

	return nil
}
