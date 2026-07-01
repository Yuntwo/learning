# 六个案例搞懂间隙锁-51CTO.COM

> [!info] Source
> https://www.51cto.com/article/779551.html

作者：码农BookSea 2024-01-16 12:19:08 [数据库](https://www.51cto.com/database) [MySQL](https://www.51cto.com/mysql) 在本文中，我们讨论了间隙锁的加锁规则。间隙锁是MySQL中用于保护范围查询和防止并发问题的重要机制，了解间隙锁的加锁规则对于优化数据库性能、减少数据冲突以及提高并发性能非常重要。

MySQL中的间隙是指索引中两个索引键之间的空间，间隙锁用于防止范围查询期间的幻读，确保查询结果的一致性和并发安全性。

## 概念解释

### 记录锁（Record Lock）

记录锁也被称为行锁，顾名思义，它是针对数据库中的行记录进行的锁定。

比如：

```
SELECT * FROM `user` WHERE `id`=1 FOR UPDATE;
```

上面的SQL会在 id=1 的行记录上加上记录锁，以阻止其他事务插入，更新，删除这一行。

### 间隙锁（Gap Lock）

间隙锁就是对间隙加锁，用于锁定索引范围之间的间隙，以避免其他事务在这个范围内插入新的数据。间隙锁是排它锁，阻止了其他事务在间隙中插入满足条件的值，间隙锁仅在可重复读隔离级别下才有效。

关于间隙锁的详细讲解放在下文，这里只是先做个概念上的介绍。

### 临键锁（Next-Key Lock）

临键锁由记录锁和间隙锁组合而成，它在索引范围内的记录上加上记录锁，并在索引范围之间的间隙上加上间隙锁。这样可以避免幻读（Phantom Read）的问题，确保事务的隔离性。

切记：间隙锁的区间是左开右开的，临键锁的区间是左开右闭的。

## 间隙锁详解

间隙锁是保证临键锁正常运作的基础，理解间隙锁的概念对于深入理解这三种锁非常重要。

间隙锁的锁定范围是指在索引范围之间的间隙

举个简单例子来说明：

假设有一个名为products的表，其中有一个整型列product\_id作为主键索引。现在有两个并发事务：事务A和事务B。

事务A执行以下语句：

```
BEGIN;
SELECT * FROM `products` WHERE `product_id` BETWEEN 100 and 200 FOR UPDATE;
```

事务B执行以下语句：

```
BEGIN;
INSERT INTO `products` (`product_id`, `name`) VALUES (150, 'Product 150');
```

在这种情况下，事务A会在products表中product\_id值在 100 和 200 之间的范围上设置间隙锁。因此，在事务A运行期间，其他事务无法在这个范围内插入新的数据，在事务B尝试插入product\_id为150的记录时，由于该记录位于事务A锁定的间隙范围内，事务B将被阻塞，直到事务A释放间隙锁为止。

### 间隙锁触发条件

在可重复读（Repeatable Read）事务隔离级别下，以下情况会产生间隙锁：

*   使用普通索引锁定：当一个事务使用普通索引进行条件查询时，MySQL会在满足条件的索引范围之间的间隙上生成间隙锁。
*   使用多列唯一索引：如果一个表存在多列组成的唯一索引，并且事务对这些列进行条件查询时，MySQL会在满足条件的索引范围之间的间隙上生成间隙锁。
*   使用唯一索引锁定多行记录：当一个事务使用唯一索引来锁定多行记录时，MySQL会在这些记录之间的间隙上生成间隙锁，以确保其他事务无法在这个范围内插入新的数据。

需要注意的是，上述情况仅在可重复读隔离级别下才会产生间隙锁。在其他隔离级别下，如读提交（Read Committed）隔离级别，MySQL可能会使用临时的意向锁来避免并发问题，而不是生成真正的间隙锁。

为什么这里强调的是普通索引呢？因为对唯一索引锁定并不会触发间隙锁，请看下面这个例子：

假设我们有一个名为students的表，其中有两个字段：id 和 name。id是主键，现在有两个事务同时进行操作：

事务A执行以下语句：

```
SELECT * FROM students WHERE id = 1 FOR UPDATE;
```

事务B执行以下语句：

```
INSERT INTO students (id, name) VALUES (2, 'John');
```

由于事务A使用了唯一索引锁定，它会锁定id为1的记录，不会触发间隙锁。同时，在事务B中插入id为2的记录也不会受到影响。这是因为唯一索引只会锁定匹配条件的具体记录，而不会锁定不存在的记录（如间隙）。

当使用唯一索引锁定一条存在的记录时，会使用记录锁，而不是间隙锁

但是当搜索条件仅涉及到多列唯一索引的一部分列时，可能会产生间隙锁。以下是一个例子：

假设students表，包含三个列：id、name和age。我们在(name, age)上创建了一个唯一索引。

现在有两个事务同时进行操作：

事务A执行以下语句：

```
SELECT * FROM students WHERE name = 'John' FOR UPDATE;
```

事务B执行以下语句：

```
INSERT INTO students (id, name, age) VALUES (2, 'John', 25);
```

在这种情况下，事务A搜索的条件只涉及到了唯一索引的一部分列（name），而没有涉及到完整的索引列（name, age）。因此，MySQL会对匹配的记录加上行锁，并且还会对与该条件范围相邻的间隙加上间隙锁。

### 间隙锁加锁规则

间隙锁有以下加锁规则：

*   规则1：加锁的基本单位是 Next-Key Lock，左开右闭区间。
*   规则2：查找过程中访问到的对象才会加锁。
*   规则3：唯一索引上的范围查询会上锁到不满足条件的第一个值为止。
*   规则4：唯一索引等值查询，并且记录存在，Next-Key Lock 退化为行锁。
*   规则5：索引上的等值查询，会将距离最近的左边界和右边界作为锁定范围，如果索引不是唯一索引还会继续向右匹配，直到遇见第一个不满足条件的值，如果最后一个值不等于查询条件，Next-Key Lock 退化为间隙锁。

记住上述这些规则，这些规则不太好理解，我们下面通过案例来讲解。

### 案例演示

环境：MySQL，InnoDB，RR隔离级别。

数据表：

```
CREATE TABLE `user` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `age` int DEFAULT NULL,
  `name` varchar(32) DEFAULT NULL,
   PRIMARY KEY (`id`)
   KEY `age` (`age`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
```

数据：

id  

age  

name  

1  

1  

小明  

5  

5  

小王  

7  

7  

小张  

11  

11  

小陈  

在进行测试之前，我们先来看看 user 表中存在的隐藏间隙：

*   (-∞, 1\]
*   (1, 5\]
*   (5, 7\]
*   (7, 11\]
*   (11, +∞\]

#### 案例一：唯一索引等值锁定存在的数据

如下是事务A和事务B执行的顺序：

时刻  

事务A  

事务B  

T1  

begin  

begin  

T2  

select \* from user where  id = 5 for update  

T3  

insert into user value(3,3,"小黑") ---不阻塞  

T4  

insert into user value(6,6,"小蓝") ---不阻塞  

T5  

commit  

commit  

根据规则4，加的是记录锁，不会使用间隙锁，所以只会锁定 5 这一行记录。

#### 案例二：索引等值锁定

时刻  

事务A  

事务B  

T1  

begin  

begin  

T2  

select \* from user where  id = 3 for update  ---  不存在的数据  

T3  

insert into user value(6,6,"小蓝")   --- 不阻塞  

T4  

insert into user value(2,2,"小黄")   --- 阻塞  

T5  

commit  

这是一个索引等值查询，根据规则1和规则5，加锁范围是（ 1，5 \] ，又由于向右遍历时最后一个值 5 不满足查询需求，Next-Key Lock 退化为间隙锁。也就是最终锁定范围区间是 （ 1，5 ）。

#### 案例三：唯一索引范围锁定

时刻  

事务A  

事务B  

T1  

begin  

begin  

T2  

select \* from user where  id >= 5 and id<6 for update  

T3  

insert into user value(7,7,"小赵")   --- 阻塞  

T4  

commit  

根据规则3，会上锁到不满足条件的第一个值为止，也就是7，所以最终加锁范围是  \[ 5，7 \]。

其实这里可以分为两个步骤，第一次用 id=5 定位记录的时候，其实加上了间隙锁 （ 1，5 \]，又因为是唯一索引等值查询，所以退化为了行锁，只锁定 5。

第二次用  id<6 定位记录的时候，其实加上了间隙锁（ 5，7 \]，所以最终合起来锁定区间是  \[ 5，7 \]。

#### 案例四：非唯一索引范围锁定

时刻  

事务A  

事务B  

T1  

begin  

begin  

T2  

select \* from user where age >= 5 and  age<6 for update  

T3  

insert into user value(8,8,"小青")   --- 不阻塞  

T4  

insert into user value(2,2,"小黄")   --- 阻塞  

T5  

commit  

参考上面那个例子。

第一次用 age =5 定位记录的时候，加上了间隙锁 （ 1，5 \]，不是唯一索引，所以不会退化为行锁，根据规则5，会继续向右匹配，所以最终合起来锁定区间是 （ 1，7 \]。

#### 案例五：间隙锁死锁

时刻  

事务A  

事务B  

T1  

begin  

begin  

T2  

select \* from user where  id = 3 for update  

T3  

select \* from user where  id = 4 for update  

T4  

insert into user value(2,2,"小黄")   --- 阻塞  

T5  

insert into user value(4,4,"小紫") ---  阻塞  

间隙锁之间不是互斥的，如果一个事务A获取到了（ 1,5 \]  之间的间隙锁，另一个事务B仍然可以获取到（ 1,5 \]  之间的间隙锁。这时就可能会发生死锁问题。

在事务A事务提交，间隙锁释放之前，事务B也获取到了间隙锁（ 1,5 \] ，这时两个事务就处于死锁状态。

#### 案例六：limit对加锁的影响

时刻  

事务A  

事务B  

T1  

begin  

begin  

T2  

deletet  user where  age = 6 limt 1  

T3  

insert into user value(7,7,"小赵")   --- 不阻塞  

T4  

T5  

commit  

commit  

根据规则5，锁定区间应该是 ( 5，7 \]，但是因为加了 limit 1 的限制，因此在遍历到 age=6 这一行之后，循环就结束了。

根据规则2，查找过程中访问到的对象才会加锁，所以最终锁定区间应该是：( 5，6 \]。

## 总结

在本文中，我们讨论了间隙锁的加锁规则。间隙锁是MySQL中用于保护范围查询和防止并发问题的重要机制，了解间隙锁的加锁规则对于优化数据库性能、减少数据冲突以及提高并发性能非常重要。

责任编辑：武晓燕 来源： [Java随想录](https://mp.weixin.qq.com/s/NBAVIhMbUq4-pUQVLy176A) [MySQL](https://so.51cto.com/?keywords=MySQL)[重要机制](https://so.51cto.com/?keywords=%E9%87%8D%E8%A6%81%E6%9C%BA%E5%88%B6)[高并发](https://so.51cto.com/?keywords=%E9%AB%98%E5%B9%B6%E5%8F%91)

---
*Generated by [Clearly Reader](https://clearlyreader.com)*