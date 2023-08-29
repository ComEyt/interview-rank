package rank

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"strconv"
	"strings"
	"time"
)

var ctx = context.Background()

type Player struct {
	Name  string
	Score int64
	Rank  int64
}

//以redis的zset作为排行榜的具体实现，创建redis的连接
type RedisClient struct {
	redisClient *redis.Client
}

// 创建连接
func NewClient() *RedisClient {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "redis_addr",
		DB:   0,
	})
	return &RedisClient{
		redisClient: redisClient,
	}
}

const (
	USERSCORE    int64 = 0
	GETSCORETIME int64 = 0
	ZSETSCORE    int64 = 0
)

// 因为得分是从大到小进行排序的，而根据题目所提示，如果分数相同则时间越小越排在前面，所以与用户分数相反。
// 所以score应该是分数与时间进行结合才可以满足提议，也就是能够从score中得到分数和当前的时间
// 所以设计userScore 为用户分数，设计getScoreTime为获取分数的时候的时间
// 所以应该设计两个函函数，一个是将score解析出来分为用户当前分数和获取当前分数的时间
// 一个是将这两个参数结合到一起，组成真正存储在zset中的score
func (r *RedisClient) decodeScore(zsetScore float64) (int64, int64) {
	// 将zset中的score进行对应的解析，解析为getScoreTime 和 userScore
	// 因为确定了最高6位的score 和 时间戳 一共合在一起的长度是16位，那么就以此长度为基本条件
	// 按照前6位进行拆分是score，如果score不够，则在前面补充上0，然后进行拆分。

	// 现将zsetScore转换为string 类型
	str := strings.Split(fmt.Sprintf("%f", zsetScore), ".")[0]
	fmt.Println(len(str))
	if len(str) != 16 {
		zeroCount := 16 - len(str)
		fmt.Println(zeroCount)
		str = strings.Repeat("0", zeroCount) + str
	}
	// 拼接完之后，分别获取前面的score 和 后面的时间戳
	userScore, _ := strconv.ParseInt(str[0:6], 10, 64)
	getScoreTime, _ := strconv.ParseInt(str[6:], 10, 64)
	
	return userScore, getScoreTime
}

func (r *RedisClient) encodeScore(userScore, getScoreTime int64) int64 {
	// 通过一些操作将userScore 和 getScoreTime进行结合，转换为真正存储在zset中的score
	// 我选择将分数添加到前6位，不足6位的分数使用0来进行区分
	uScore := fmt.Sprintf("%06d", userScore)
	// getScoreTime 是当前的unix时间戳，将两个进行拼接之后转换为int类型
	time := strconv.Itoa(int(getScoreTime))
	result, _ := strconv.ParseInt(uScore+time, 10, 64)
	return result
}

// 向redis中添加得分
func (r *RedisClient) UpdateScore(score int64, name string) error {
	existingScore, err := r.redisClient.ZScore(ctx, "leaderboard", name).Result()
	if err != nil && err != redis.Nil {
		return err
	}

	var uScore int64
	// 判断是否存在这个name对应的socre值
	if existingScore == 0 {
		// 如果不存在，则传入的分数就是用户当前的分数。
		uScore = score
	} else {
		// 如果存在，则将当前的分数进行解析，解析为具体分数和添加时间，然后直接将分数与对应的当前分数进行添加。
		uScore, _ = r.decodeScore(existingScore)
		uScore += score
	}

	// 接下来是修改对应的getScoreTime字段，然后使用上述中对应参数，直接将newScore生成出来，然后存储到当前分数上
	gotScoreTime := time.Now().Unix()
	newScore := r.encodeScore(uScore, gotScoreTime)

	_, err = r.redisClient.ZAdd(ctx, "leaderboard", &redis.Z{
		Score:  float64(newScore),
		Member: name,
	}).Result()

	return err
}

// 获取整个用户在当前排行榜中的排名
func (r *RedisClient) GetUserRank(name string) (int64, error) {
	rank, err := r.redisClient.ZRevRank(ctx, "leaderboard", name).Result()
	if err != nil {
		return -1, err
	}
	return rank + 1, nil
}

// 获取用户前后10位的分数和排名
func (r *RedisClient) GetUserRankAndTopPlayers(name string) ([]Player, error) {
	// 获取我的rank
	myRank, err := r.redisClient.ZRevRank(ctx, "leaderboard", name).Result()
	if err != nil {
		return nil, err
	}

	// 获取前十和后10名人的rank
	topRank := myRank - 10
	if topRank < 0 {
		topRank = 0
	}

	endRank := myRank + 10
	topPlayers, err := r.redisClient.ZRevRangeWithScores(ctx, "leaderboard", topRank, endRank).Result()
	if err != nil {
		return nil, err
	}
	var result []Player
	for i, v := range topPlayers {
		player := Player{
			Name:  v.Member.(string),
			Score: int64(v.Score),
			Rank:  topRank + int64(i) + 1,
		}
		result = append(result, player)
	}

	return result, nil
}
